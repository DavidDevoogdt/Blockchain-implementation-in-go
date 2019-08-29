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

	i uint32

	RecieveChannel chan []byte
}

//InitializeTransactionPool needs to be called once for initialization of the data structure
func (m *Miner) InitializeTransactionPool() *TransactionPool {
	tp := new(TransactionPool)
	tp.IncomingRequests = make(map[[32]byte]*TransactionBlock, 0)
	tp.SignatureVerifiedRequest = make(map[[32]byte]*TransactionBlock, 0)
	tp.RecieveChannel = make(chan []byte)
	tp.miner = m
	go tp.ReceiveTransactionBlock()

	return tp
}

// ReceiveTransactionBlock verifies and adds a tx to the pool of blocks for the next transactionblockgroup
func (tp *TransactionPool) ReceiveTransactionBlock() {
	tbArr := <-tp.RecieveChannel
	//todo at the time only complete good blocks according to current blockchain status get into the signatureverifiedpool
	go func() {
		tb := DeserializeTransactionBlock(tbArr, tp.miner.BlockChain)
		if tb.NumberOfUnresolvedReferences != uint8(0) {
			fmt.Printf("todo complete references or reject block")
		}
		good := tb.VerifyExceptUTXO(false)
		if good {
			tp.SignatureVerifiedMutex.Lock()
			tp.numberSignatureVerified++
			tp.SignatureVerifiedRequest[tb.Hash()] = tb
			tp.SignatureVerifiedMutex.Unlock()
		} else {
			fmt.Printf("todo receive incomplete block")
		}
	}()

}

// GenerateTransctionBlockGroup makes a fee block and adds the blocks in the pool to a full transactionblockgroup
func (tp *TransactionPool) GenerateTransctionBlockGroup() *TransactionBlockGroup {

	//m := tp.miner

	feeblock := tp.GenerateFeeBlock()
	tbg := InitializeTransactionBlockGroup()

	tbg.Add(feeblock)

	for _, vr := range tp.SignatureVerifiedRequest {
		tbg.Add(vr)
	}

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
	m := tp.miner

	m.DebugPrint(" started preparing tbg for mining\n")

	m.GeneralMutex.Lock()
	bc := m.BlockChain
	m.GeneralMutex.Unlock()

	bc.BlockChainMutex.Lock()
	head := bc.Head
	bc.BlockChainMutex.Unlock()

	head.generalMutex.RLock()
	headUTxOManagerIsUpToDate := head.UTxOManagerIsUpToDate
	headUTxOManagerPointer := head.UTxOManagerPointer
	prevBlock := head.Block
	head.generalMutex.RUnlock()

	if !headUTxOManagerIsUpToDate {
		m.DebugPrint(fmt.Sprintf("head is not up to date, invoking and returning\n"))
	}

	goodSign := make(chan bool)
	goodRef := make(chan bool)

	go func() {
		goodSign <- tbg.VerifyExceptUTXO()
	}()

	go func() {
		goodRef <- headUTxOManagerPointer.VerifyTransactionBlockRefs(tbg)
	}()

	newBlock := new(Block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = rand.Uint32()
	newBlock.MerkleRoot = tbg.merkleTree.GetMerkleRoot()
	newBlock.Difficulty = prevBlock.Difficulty

	if !(<-goodRef) {
		m.DebugPrint("The prepared transactionblockgroup for mining had bad refs, ignoring\n")
		return nil
	}

	if !(<-goodSign) {
		m.DebugPrint("The prepared transactionblockgroup bad signatures, ignoring\n")
		return nil
	}

	//m.DebugPrint("finished praparation of block\n")

	return newBlock
}
