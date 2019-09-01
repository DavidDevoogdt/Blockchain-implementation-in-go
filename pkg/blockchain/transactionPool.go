package davidcoin

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	rsautil "project/pkg/rsa_util"
	"sync"
)

// TransactionPool is Datatype that keeps track of al the requested transaction and generates a transactionblockgroup for the miner
type TransactionPool struct {
	Miner *Miner

	incomingMutex    sync.RWMutex
	numberIncoming   uint16
	IncomingRequests map[[32]byte]*transactionBlock

	signatureVerifiedMutex   sync.RWMutex
	numberSignatureVerified  uint16
	SignatureVerifiedRequest map[[32]byte]*transactionBlock

	priorityMutex   sync.RWMutex
	numberPriority  uint16
	priorityRequest map[[32]byte]*transactionBlock

	i uint32

	recieveChannel chan []byte
}

//updateWithBlockchainNode checks whether some transactionrequest refer to this block
//also remove psent transactions
func (tp *TransactionPool) updateWithBlockchainNode(bcn *BlockChainNode) {
	bcn.generalMutex.Lock()
	bcn.knownToTransactionPool = true
	_prevBCN := bcn.previousBlockChainNode
	bcn.generalMutex.Unlock()

	_prevBCN.generalMutex.RLock()
	ok := _prevBCN.knownToTransactionPool
	_prevBCN.generalMutex.RUnlock()

	if !ok {
		tp.updateWithBlockchainNode(_prevBCN)
	}

	bcn.generalMutex.Lock()
	dp := bcn.dataPointer
	bcn.generalMutex.Unlock()

	if dp == nil {
		tp.Miner.DebugPrint("tp updating but no data, error\n")
		return
	}

	for _, bl := range dp.TransactionBlockStructs {
		h := bl.hash()
		tp.signatureVerifiedMutex.Lock()
		if _, ok := tp.SignatureVerifiedRequest[h]; ok {
			delete(tp.SignatureVerifiedRequest, h)
			tp.Miner.DebugPrint(fmt.Sprintf("%s removed transaction from pool because it is spent\n", tp.Miner.Name))
		}
		tp.signatureVerifiedMutex.Unlock()

		tp.priorityMutex.Lock()
		if _, ok := tp.priorityRequest[h]; ok {
			delete(tp.priorityRequest, h)
			tp.Miner.DebugPrint(fmt.Sprintf("%s removed priority transaction from pool because it is spent\n", tp.Miner.Name))
		}
		tp.priorityMutex.Unlock()

	}

	//todo verify if any incomingrequest can be solved with the new block
}

//initializeTransactionPool needs to be called once for initialization of the data structure
func (m *Miner) initializeTransactionPool() *TransactionPool {
	tp := new(TransactionPool)
	tp.IncomingRequests = make(map[[32]byte]*transactionBlock, 0)
	tp.SignatureVerifiedRequest = make(map[[32]byte]*transactionBlock, 0)
	tp.priorityRequest = make(map[[32]byte]*transactionBlock, 0)
	tp.recieveChannel = make(chan []byte)
	tp.Miner = m
	go tp.receiveTransactionBlock()

	return tp
}

// receiveTransactionBlock verifies and adds a tx to the pool of blocks for the next transactionblockgroup
func (tp *TransactionPool) receiveTransactionBlock() {
	for {
		tbArr := <-tp.recieveChannel
		//todo at the time only complete good blocks according to current blockchain status get into the signatureverifiedpool
		go func() {
			tb := deserializeTransactionBlock(tbArr, tp.Miner.BlockChain)
			if tb.NumberOfUnresolvedReferences != uint8(0) {
				tp.Miner.DebugPrint(fmt.Sprintf("todo complete references or reject block\n"))
			}
			good := tb.verifyExceptUTXO(false)
			if good {
				tp.signatureVerifiedMutex.Lock()
				tp.numberSignatureVerified++
				tp.SignatureVerifiedRequest[tb.hash()] = tb
				tp.signatureVerifiedMutex.Unlock()
				tp.Miner.DebugPrint(fmt.Sprintf("%s added extra transaction to pool\n", tp.Miner.Name))
			} else {
				tp.Miner.DebugPrint(fmt.Sprintf("block had bad utxo ref\n"))
			}
		}()
	}
}

// receiveInternal skips all checks
func (tp *TransactionPool) receiveInternal(tb *transactionBlock) {
	tp.priorityMutex.Lock()
	tp.numberPriority++
	tp.priorityRequest[tb.hash()] = tb
	tp.priorityMutex.Unlock()
}

// generateTransctionBlockGroup makes a fee block and adds the blocks in the pool to a full transactionblockgroup
func (tp *TransactionPool) generateTransctionBlockGroup() *transactionBlockGroup {

	//m := tp.miner

	feeblock := tp.generateFeeBlock()
	tbg := initializeTransactionBlockGroup()

	tbg.Add(feeblock)

	bc := tp.Miner.BlockChain

	bc.blockChainMutex.Lock()
	head := bc.head
	bc.blockChainMutex.Unlock()

	head.generalMutex.RLock()
	utxo := head.uTxOManagerPointer
	head.generalMutex.RUnlock()

	if utxo == nil {
		tp.Miner.DebugPrint(fmt.Sprintf("todo implement non up to date head\n"))
		return nil
	}

	tp.priorityMutex.RLock()
	for _, vr := range tp.priorityRequest {
		tbg.Add(vr)
	}
	tp.priorityMutex.RUnlock()

	tp.signatureVerifiedMutex.RLock()
	for _, vr := range tp.SignatureVerifiedRequest {

		if utxo.VerifyTransactionBlockRefs(vr) {
			tp.Miner.DebugPrint(fmt.Sprintf("%s added extra transaction to tbg\n", tp.Miner.Name))
			tbg.Add(vr)
		} else {
			tp.Miner.DebugPrint("block is transactionpool is bad\n")
		}

	}
	tp.signatureVerifiedMutex.RUnlock()

	tbg.finalizeTransactionBlockGroup()

	return tbg
}

// generateFeeBlock sets up a fee transaction according to the rules
func (tp *TransactionPool) generateFeeBlock() *transactionBlock {
	m := tp.Miner
	tp.i++
	tb := initializeTransactionBlock()
	copy(tb.PayerPublicKey[0:rsautil.KeySize], m.Wallet.PublicKey[0:rsautil.KeySize])

	to := &transactionOutput{Amount: 1 * Davidcoin}
	copy(to.ReceiverPublicKey[0:rsautil.KeySize], m.Wallet.PublicKey[0:rsautil.KeySize])
	binary.LittleEndian.PutUint32(to.text[0:4], tp.i)
	to.sign(m.Wallet.privateKey)

	tb.AddOutput(to)
	tb.signBlock(m.Wallet.privateKey)

	return tb
}

// prepareBlockForMining takes transactiondata and builds a block suitable for mining
func (tp *TransactionPool) prepareBlockForMining(tbg *transactionBlockGroup) *block {
	if tbg == nil {
		return nil
	}

	m := tp.Miner

	m.DebugPrint(" started preparing tbg for mining\n")

	m.generalMutex.Lock()
	bc := m.BlockChain
	m.generalMutex.Unlock()

	bc.blockChainMutex.Lock()
	head := bc.head
	bc.blockChainMutex.Unlock()

	head.generalMutex.RLock()
	prevBlock := head.Block
	head.generalMutex.RUnlock()

	newBlock := new(block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = rand.Uint32()
	newBlock.MerkleRoot = tbg.merkleTree.GetMerkleRoot()
	newBlock.Difficulty = prevBlock.Difficulty

	return newBlock
}
