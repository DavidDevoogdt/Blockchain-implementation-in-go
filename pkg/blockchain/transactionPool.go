package davidcoin

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	rsautil "project/pkg/rsa_util"
	"sync"
)

// transactionPool is Datatype that keeps track of al the requested transaction and generates a transactionblockgroup for the miner
type transactionPool struct {
	miner *Miner

	incomingMutex    sync.RWMutex
	numberIncoming   uint16
	incomingRequests map[[32]byte]*transactionBlock

	signatureVerifiedMutex   sync.RWMutex
	numberSignatureVerified  uint16
	signatureVerifiedRequest map[[32]byte]*transactionBlock

	priorityMutex   sync.RWMutex
	numberPriority  uint16
	priorityRequest map[[32]byte]*transactionBlock

	i uint32

	recieveChannel chan []byte
}

//updateWithBlockchainNode checks whether some transactionrequest refer to this block
//also remove psent transactions
func (tp *transactionPool) updateWithBlockchainNode(bcn *BlockChainNode) {
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
		tp.miner.debugPrint("tp updating but no data, error\n")
		return
	}

	for _, bl := range dp.TransactionBlockStructs {
		h := bl.hash()
		tp.signatureVerifiedMutex.Lock()
		if _, ok := tp.signatureVerifiedRequest[h]; ok {
			delete(tp.signatureVerifiedRequest, h)
			tp.miner.debugPrint(fmt.Sprintf("%s removed transaction from pool because it is spent\n", tp.miner.Name))
		}
		tp.signatureVerifiedMutex.Unlock()

		tp.priorityMutex.Lock()
		if _, ok := tp.priorityRequest[h]; ok {
			delete(tp.priorityRequest, h)
			tp.miner.debugPrint(fmt.Sprintf("%s removed priority transaction from pool because it is spent\n", tp.miner.Name))
		}
		tp.priorityMutex.Unlock()

	}

	//todo verify if any incomingrequest can be solved with the new block
}

//initializeTransactionPool needs to be called once for initialization of the data structure
func (m *Miner) initializeTransactionPool() *transactionPool {
	tp := new(transactionPool)
	tp.incomingRequests = make(map[[32]byte]*transactionBlock, 0)
	tp.signatureVerifiedRequest = make(map[[32]byte]*transactionBlock, 0)
	tp.priorityRequest = make(map[[32]byte]*transactionBlock, 0)
	tp.recieveChannel = make(chan []byte)
	tp.miner = m
	go tp.receiveTransactionBlock()

	return tp
}

// receiveTransactionBlock verifies and adds a tx to the pool of blocks for the next transactionblockgroup
func (tp *transactionPool) receiveTransactionBlock() {
	for {
		tbArr := <-tp.recieveChannel
		//todo at the time only complete good blocks according to current blockchain status get into the signatureverifiedpool
		go func() {
			tb := deserializeTransactionBlock(tbArr, tp.miner.BlockChain)
			if tb.NumberOfUnresolvedReferences != uint8(0) {
				tp.miner.debugPrint(fmt.Sprintf("todo complete references or reject block\n"))
			}
			good := tb.verifyExceptUTXO(false)
			if good {
				tp.signatureVerifiedMutex.Lock()
				tp.numberSignatureVerified++
				tp.signatureVerifiedRequest[tb.hash()] = tb
				tp.signatureVerifiedMutex.Unlock()
				tp.miner.debugPrint(fmt.Sprintf("%s added extra transaction to pool\n", tp.miner.Name))
			} else {
				tp.miner.debugPrint(fmt.Sprintf("block had bad utxo ref\n"))
			}
		}()
	}
}

// receiveInternal skips all checks
func (tp *transactionPool) receiveInternal(tb *transactionBlock) {
	tp.priorityMutex.Lock()
	tp.numberPriority++
	tp.priorityRequest[tb.hash()] = tb
	tp.priorityMutex.Unlock()
}

// generateTransctionBlockGroup makes a fee block and adds the blocks in the pool to a full transactionblockgroup
func (tp *transactionPool) generateTransctionBlockGroup() *transactionBlockGroup {

	//m := tp.miner

	feeblock := tp.generateFeeBlock()
	tbg := initializeTransactionBlockGroup()

	tbg.Add(feeblock)

	bc := tp.miner.BlockChain

	bc.blockChainMutex.Lock()
	head := bc.head
	bc.blockChainMutex.Unlock()

	head.generalMutex.RLock()
	utxo := head.uTxOManagerPointer
	head.generalMutex.RUnlock()

	if utxo == nil {
		tp.miner.debugPrint(fmt.Sprintf("todo implement non up to date head\n"))
		return nil
	}

	tp.priorityMutex.RLock()
	for _, vr := range tp.priorityRequest {
		tbg.Add(vr)
	}
	tp.priorityMutex.RUnlock()

	tp.signatureVerifiedMutex.RLock()
	for _, vr := range tp.signatureVerifiedRequest {

		if utxo.VerifyTransactionBlockRefs(vr) {
			tp.miner.debugPrint(fmt.Sprintf("%s added extra transaction to tbg\n", tp.miner.Name))
			tbg.Add(vr)
		} else {
			tp.miner.debugPrint("block is transactionpool is bad\n")
		}

	}
	tp.signatureVerifiedMutex.RUnlock()

	tbg.finalizeTransactionBlockGroup()

	return tbg
}

// generateFeeBlock sets up a fee transaction according to the rules
func (tp *transactionPool) generateFeeBlock() *transactionBlock {
	m := tp.miner
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
func (tp *transactionPool) prepareBlockForMining(tbg *transactionBlockGroup) *block {
	if tbg == nil {
		return nil
	}

	m := tp.miner

	m.debugPrint(" started preparing tbg for mining\n")

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
