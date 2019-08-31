package davidcoin

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
)

// TransactionPool is Datatype that keeps track of al the requested transaction and generates a transactionblockgroup for the miner
type TransactionPool struct {
	miner *Miner

	incomingMutex    sync.RWMutex
	numberIncoming   uint16
	IncomingRequests map[[32]byte]*TransactionBlock

	SignatureVerifiedMutex   sync.RWMutex
	numberSignatureVerified  uint16
	SignatureVerifiedRequest map[[32]byte]*TransactionBlock

	priorityMutex   sync.RWMutex
	numberPriority  uint16
	PriorityRequest map[[32]byte]*TransactionBlock

	i uint32

	RecieveChannel chan []byte
}

//UpdateWithBlockchainNode checks whether some transactionrequest refer to this block
//also remove psent transactions
func (tp *TransactionPool) UpdateWithBlockchainNode(bcn *BlockChainNode) {
	bcn.generalMutex.Lock()
	bcn.knownToTransactionPool = true
	_prevBCN := bcn.PrevBlockChainNode
	bcn.generalMutex.Unlock()

	_prevBCN.generalMutex.RLock()
	ok := _prevBCN.knownToTransactionPool
	_prevBCN.generalMutex.RUnlock()

	if !ok {
		tp.UpdateWithBlockchainNode(_prevBCN)
	}

	bcn.generalMutex.Lock()
	dp := bcn.DataPointer
	bcn.generalMutex.Unlock()

	if dp == nil {
		tp.miner.DebugPrint("tp updating but no data, error\n")
		return
	}

	for _, bl := range dp.TransactionBlockStructs {
		h := bl.Hash()
		tp.SignatureVerifiedMutex.Lock()
		if _, ok := tp.SignatureVerifiedRequest[h]; ok {
			delete(tp.SignatureVerifiedRequest, h)
			tp.miner.DebugPrint(fmt.Sprintf("%s removed transaction from pool because it is spent\n", tp.miner.Name))
		}
		tp.SignatureVerifiedMutex.Unlock()

		tp.priorityMutex.Lock()
		if _, ok := tp.PriorityRequest[h]; ok {
			delete(tp.PriorityRequest, h)
			tp.miner.DebugPrint(fmt.Sprintf("%s removed priority transaction from pool because it is spent\n", tp.miner.Name))
		}
		tp.priorityMutex.Unlock()

	}

	//todo verify if any incomingrequest can be solved with the new block
}

//InitializeTransactionPool needs to be called once for initialization of the data structure
func (m *Miner) InitializeTransactionPool() *TransactionPool {
	tp := new(TransactionPool)
	tp.IncomingRequests = make(map[[32]byte]*TransactionBlock, 0)
	tp.SignatureVerifiedRequest = make(map[[32]byte]*TransactionBlock, 0)
	tp.PriorityRequest = make(map[[32]byte]*TransactionBlock, 0)
	tp.RecieveChannel = make(chan []byte)
	tp.miner = m
	go tp.ReceiveTransactionBlock()

	return tp
}

// ReceiveTransactionBlock verifies and adds a tx to the pool of blocks for the next transactionblockgroup
func (tp *TransactionPool) ReceiveTransactionBlock() {
	for {
		tbArr := <-tp.RecieveChannel
		//todo at the time only complete good blocks according to current blockchain status get into the signatureverifiedpool
		go func() {
			tb := DeserializeTransactionBlock(tbArr, tp.miner.BlockChain)
			if tb.NumberOfUnresolvedReferences != uint8(0) {
				tp.miner.DebugPrint(fmt.Sprintf("todo complete references or reject block\n"))
			}
			good := tb.VerifyExceptUTXO(false)
			if good {
				tp.SignatureVerifiedMutex.Lock()
				tp.numberSignatureVerified++
				tp.SignatureVerifiedRequest[tb.Hash()] = tb
				tp.SignatureVerifiedMutex.Unlock()
				tp.miner.DebugPrint(fmt.Sprintf("%s added extra transaction to pool\n", tp.miner.Name))
			} else {
				tp.miner.DebugPrint(fmt.Sprintf("block had bad utxo ref\n"))
			}
		}()
	}
}

// ReceiveInternal skips all checks
func (tp *TransactionPool) ReceiveInternal(tb *TransactionBlock) {
	tp.priorityMutex.Lock()
	tp.numberPriority++
	tp.PriorityRequest[tb.Hash()] = tb
	tp.priorityMutex.Unlock()
}

// GenerateTransctionBlockGroup makes a fee block and adds the blocks in the pool to a full transactionblockgroup
func (tp *TransactionPool) GenerateTransctionBlockGroup() *TransactionBlockGroup {

	//m := tp.miner

	feeblock := tp.GenerateFeeBlock()
	tbg := InitializeTransactionBlockGroup()

	tbg.Add(feeblock)

	bc := tp.miner.BlockChain

	bc.BlockChainMutex.Lock()
	head := bc.Head
	bc.BlockChainMutex.Unlock()

	head.generalMutex.RLock()
	utxo := head.UTxOManagerPointer
	head.generalMutex.RUnlock()

	if utxo == nil {
		tp.miner.DebugPrint(fmt.Sprintf("todo implement non up to date head\n"))
		return nil
	}

	tp.priorityMutex.RLock()
	for _, vr := range tp.PriorityRequest {
		tbg.Add(vr)
	}
	tp.priorityMutex.RUnlock()

	tp.SignatureVerifiedMutex.RLock()
	for _, vr := range tp.SignatureVerifiedRequest {

		if utxo.VerifyTransactionBlockRefs(vr) {
			tp.miner.DebugPrint(fmt.Sprintf("%s added extra transaction to tbg\n", tp.miner.Name))
			tbg.Add(vr)
		} else {
			tp.miner.DebugPrint("block is transactionpool is bad\n")
		}

	}
	tp.SignatureVerifiedMutex.RUnlock()

	tbg.FinalizeTransactionBlockGroup()

	return tbg
}

// GenerateFeeBlock sets up a fee transaction according to the rules
func (tp *TransactionPool) GenerateFeeBlock() *TransactionBlock {
	m := tp.miner
	tp.i++
	tb := InitializeTransactionBlock()
	copy(tb.PayerPublicKey[0:KeySize], m.Wallet.PublicKey[0:KeySize])

	to := &TransactionOutput{Amount: 1 * davidcoin}
	copy(to.ReceiverPublicKey[0:KeySize], m.Wallet.PublicKey[0:KeySize])
	binary.LittleEndian.PutUint32(to.text[0:4], tp.i)
	to.Sign(m.Wallet.PrivateKey)

	tb.AddOutput(to)
	tb.SignBlock(m.Wallet.PrivateKey)

	return tb
}

// PrepareBlockForMining takes transactiondata and builds a block suitable for mining
func (tp *TransactionPool) PrepareBlockForMining(tbg *TransactionBlockGroup) *Block {
	if tbg == nil {
		return nil
	}

	m := tp.miner

	m.DebugPrint(" started preparing tbg for mining\n")

	m.GeneralMutex.Lock()
	bc := m.BlockChain
	m.GeneralMutex.Unlock()

	bc.BlockChainMutex.Lock()
	head := bc.Head
	bc.BlockChainMutex.Unlock()

	head.generalMutex.RLock()
	prevBlock := head.Block
	head.generalMutex.RUnlock()

	newBlock := new(Block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = rand.Uint32()
	newBlock.MerkleRoot = tbg.merkleTree.GetMerkleRoot()
	newBlock.Difficulty = prevBlock.Difficulty

	return newBlock
}
