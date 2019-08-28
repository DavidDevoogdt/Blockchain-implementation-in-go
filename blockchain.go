package main

import (
	"fmt"
	"log"
	"sync"
)

/*
rules for manipulation:
blockchainnodes need to be writen by asynchronous request to blockchainwriter to avoid deadlock
the blockchainwrite will return with the channel in the struct
Each function claims and releases its own locks, during locking no external function can be called synchronously

for nested functions:
______________________

ClosingChans := make([]chan bool, 0)

read state, make sure nothing blocks
...

to write:

_BlockChan := make(chan bool)
_Block.writeRequest <- WriteRequest{
	func() {
		// do stuf with _block
	}, _BlockChan}

ClosingChans = append(ClosingChans, _BlockChan)

....
after, close all readlocks
for _, c := range ClosingChans {
	<-c
}

*/

// BlockChain is keeps track of known heads of tree
type BlockChain struct {
	Head              *BlockChainNode
	Root              *BlockChainNode
	OrphanBlockChains []*BlockChain
	IsOrphan          bool
	BlockChainMutex   sync.RWMutex

	utxoChan         chan *BlockChainNode
	AllNodesMap      map[[32]byte]*BlockChainNode
	AllNodesMapMutex sync.Mutex

	OtherHeadNodes      map[[32]byte]*BlockChainNode
	OtherHeadNodesMutex sync.Mutex

	DanglingData      map[[32]byte]*TransactionBlockGroup // indexed by merkle root, not hash
	DanglingDataMutex sync.RWMutex

	MerkleRootToBlockHashMap      map[[32]byte][32]byte
	MerkleRootToBlockHashMapMutex sync.RWMutex
	// const
	Miner *Miner

	Height uint32
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	generalMutex       sync.RWMutex
	PrevBlockChainNode *BlockChainNode
	NextBlockChainNode *BlockChainNode
	Block              *Block
	Hash               [32]byte

	HasData     bool
	DataPointer *TransactionBlockGroup

	UTxOManagerPointer      *UTxOManager
	UTxOManagerIsUpToDate   bool
	VerifiedWithUtxoManager bool
	Badblock                bool // true if verified bad

	dataHasArrived chan bool

	//writeRequest chan WriteRequest
}

/*
type WriteRequest struct {
	f func()
	c chan bool
}

// BlockChainNodeUpdater All the writes are performed from this function run in a separate goroutine
func (bcn *BlockChainNode) BlockChainNodeUpdater() {
	for {
		s, ok := <-bcn.writeRequest
		bcn.generalMutex.Lock()
		s.f()
		bcn.generalMutex.Unlock()
		go func() {
			s.c <- ok
		}()
	}
}*/

// HasBlock determines whether block is in chain
func (bc *BlockChain) HasBlock(hash [32]byte) bool {
	bc.AllNodesMapMutex.Lock()
	_, ok := bc.AllNodesMap[hash]
	bc.AllNodesMapMutex.Unlock()
	return ok
}

// GetBlockChainNodeAtHeight returns block of main chain at specific height
func (bc *BlockChain) GetBlockChainNodeAtHeight(height uint32) *BlockChainNode {
	//bc.BlockChainMutex.RLock()
	//defer bc.BlockChainMutex.RUnlock()

	current := bc.Head

	max := current.Block.BlockCount

	if height > max {
		return nil
	}

	for i := uint32(0); i < max-height; i++ {

		current.generalMutex.RLock()
		defer current.generalMutex.RUnlock()
		current = current.PrevBlockChainNode
	}

	//bc.Miner.DebugPrint(("requested %d, got %d", height, current.Block.BlockCount)

	return current
}

// GetBlockChainNodeAtHash fetches block
func (bc *BlockChain) GetBlockChainNodeAtHash(hash [32]byte) *BlockChainNode {
	bc.AllNodesMapMutex.Lock()
	val, ok := bc.AllNodesMap[hash]
	bc.AllNodesMapMutex.Unlock()

	if ok {
		return val
	}

	bc.BlockChainMutex.Lock()
	for _, obc := range bc.OrphanBlockChains {
		bc.BlockChainMutex.Unlock()
		val := obc.GetBlockChainNodeAtHash(hash)
		if val != nil {
			return val
		}
		bc.BlockChainMutex.Lock()
	}
	bc.BlockChainMutex.Unlock()

	return nil
}

// GetTransactionOutput turns reference into locally saved version
func (bc *BlockChain) GetTransactionOutput(tr *TransactionRef) *TransactionOutput {
	// todo verifiy this data is present
	return bc.AllNodesMap[tr.BlockHash].DataPointer.TransactionBlockStructs[tr.TransactionBlockNumber].OutputList[tr.OutputNumber]
}

// return bool specifies whether miner should stop immediately
// has no nested locks
func (bc *BlockChain) addBlockChainNode(block0 *Block) {

	// only request blockchain lock directly

	if !block0.Verify() {
		bc.Miner.DebugPrint(fmt.Sprintf("Block not ok, not added\n"))
		return
	}
	// setup space for function which should be called after all defers, ie after all locks are freed
	Endfunctions := make([]func(), 0)
	defer func() {
		for _, f := range Endfunctions {

			f()
		}
	}()

	bc.BlockChainMutex.RLock()
	bcHead := bc.Head
	bcIsOrphan := bc.IsOrphan
	bcMiner := bc.Miner
	bc.BlockChainMutex.RUnlock()

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash
	_Merkle := block0.MerkleRoot

	bc.AllNodesMapMutex.Lock()
	_, nok := bc.AllNodesMap[block0.Hash()]
	bc.AllNodesMapMutex.Unlock()

	if nok {
		bc.Miner.DebugPrint(fmt.Sprintf("already in chain\n"))
		return
	}

	bc.MerkleRootToBlockHashMapMutex.Lock()
	bc.MerkleRootToBlockHashMap[_Merkle] = _Hash
	bc.MerkleRootToBlockHashMapMutex.Unlock()

	bc.AllNodesMapMutex.Lock()
	prevBlock, ok := bc.AllNodesMap[_PrevHash]
	bc.AllNodesMapMutex.Unlock()

	if !ok {

		for _, obc := range bc.OrphanBlockChains[:] {
			obc.BlockChainMutex.Lock()
			defer obc.BlockChainMutex.Unlock()

			if obc.HasBlock(_PrevHash) {
				bcMiner.DebugPrint(fmt.Sprintf("block added to orphaned chain \n"))

				Endfunctions = append(Endfunctions, func() {
					obc.addBlockChainNode(block0)
				})
				return
			}
		}

		// check whether hash is prevhash of orphan chain root
		for _, obc := range bc.OrphanBlockChains[:] {

			if obc.Root.Block.PrevHash == _Hash {
				newBCN := new(BlockChainNode)
				newBCN.Block = block0
				newBCN.Hash = _Hash
				newBCN.dataHasArrived = make(chan bool)
				obc.Root.PrevBlockChainNode = newBCN
				newBCN.NextBlockChainNode = obc.Root
				obc.Root = newBCN

				bc.AllNodesMapMutex.Lock()
				obc.AllNodesMap[_Hash] = newBCN
				bc.AllNodesMapMutex.Unlock()

				bcMiner.DebugPrint(fmt.Sprintf("setting new root for orphaned chain \n"))

				go bcMiner.RequestBlockFromHash(block0.PrevHash)

				return
			}
		}

		bcMiner.DebugPrint(fmt.Sprintf("prev Block not in history, requesting %.10x\n", _PrevHash))
		bcMiner.DebugPrint(fmt.Sprintf("creating new orphaned chain \n"))

		bc.BlockChainMutex.Lock()
		bc.OrphanBlockChains = append(bc.OrphanBlockChains, bc.InitializeOrphanBlockChain(block0))
		bc.BlockChainMutex.Unlock()

		go bc.Miner.RequestBlockFromHash(block0.PrevHash)

		return

	}

	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
	newBCN.dataHasArrived = make(chan bool)
	newBCN.HasData = false
	newBCN.UTxOManagerIsUpToDate = false

	bc.AllNodesMapMutex.Lock()
	bc.AllNodesMap[_Hash] = newBCN
	bc.AllNodesMapMutex.Unlock()

	prevBlock.generalMutex.Lock()
	prevBlock.NextBlockChainNode = newBCN
	prevBlock.generalMutex.Unlock()

	newBCN.PrevBlockChainNode = prevBlock

	// check whether hash is contained as prehash in orphaned chains
	for i, obcR := range bc.OrphanBlockChains {

		obc := obcR

		obc.Root.generalMutex.Lock()

		if _Hash == obc.Root.Block.PrevHash {
			//defer obc.BlockChainMutex.Unlock()

			obc.Root.PrevBlockChainNode = newBCN
			obcHead := obc.Head
			obc.Root.generalMutex.Unlock()

			bcMiner.DebugPrint(fmt.Sprintf("joining orphaned chain to main chain!\n"))
			// delete prev node from head if that were the case

			bc.OtherHeadNodesMutex.Lock()
			delete(bc.OtherHeadNodes, _PrevHash)
			for k, v := range obc.OtherHeadNodes { //merge hashmaps
				bc.OtherHeadNodes[k] = v
			}
			bc.OtherHeadNodesMutex.Unlock()

			bc.AllNodesMapMutex.Lock()
			for k, v := range obc.AllNodesMap {
				bc.AllNodesMap[k] = v
			}
			bc.AllNodesMapMutex.Unlock()

			// put orphaned blockchain to last element and remove it
			l := len(bc.OrphanBlockChains)

			bc.BlockChainMutex.Lock()
			bc.OrphanBlockChains[i] = bc.OrphanBlockChains[l-1]
			bc.OrphanBlockChains = bc.OrphanBlockChains[:l-1]
			bc.BlockChainMutex.Unlock()

			orphanHead := obc.Head

			obcHead.generalMutex.RLock()
			olen := orphanHead.Block.BlockCount
			obcHead.generalMutex.RUnlock()

			bcHead.generalMutex.RLock()
			blen := bcHead.Block.BlockCount
			bcHead.generalMutex.RUnlock()

			if olen > blen {
				bc.Miner.DebugPrint(fmt.Sprintf("orphaned chain is now main chain\n"))

				Endfunctions = append(Endfunctions, func() {
					go func() { bc.utxoChan <- orphanHead }()
				})
			}

			return
		}

		obc.Root.generalMutex.Unlock()
	}

	//nothing special, just main chain

	bc.OtherHeadNodesMutex.Lock()
	_, ok = bc.OtherHeadNodes[_PrevHash]
	bc.OtherHeadNodesMutex.Unlock()

	if ok {
		bcMiner.DebugPrint(fmt.Sprintf("added to head chain \n"))

		bc.OtherHeadNodesMutex.Lock()
		delete(bc.OtherHeadNodes, _PrevHash)
		bc.OtherHeadNodes[_Hash] = newBCN
		bc.OtherHeadNodesMutex.Unlock()

	} else {

		bc.OtherHeadNodesMutex.Lock()
		bc.OtherHeadNodes[newBCN.Hash] = newBCN
		bc.OtherHeadNodesMutex.Unlock()
		bcMiner.DebugPrint(fmt.Sprintf("new fork created \n"))
	}

	bcHead.generalMutex.Lock()
	hlen := bcHead.Block.BlockCount

	if hlen < newBCN.Block.BlockCount {
		if bcIsOrphan {
			bc.Head = newBCN
		} else {
			Endfunctions = append(Endfunctions, func() {
				go func() { bc.utxoChan <- newBCN }()
			})

		}

	}

	bcHead.generalMutex.Unlock()

}

//AddInternal is used to add mined block and data at once
func (bc *BlockChain) AddInternal(block0 *Block, data *TransactionBlockGroup) {
	// todo add checks

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash

	bc.AllNodesMapMutex.Lock()
	prevBlock := bc.AllNodesMap[_PrevHash]
	bc.AllNodesMapMutex.Unlock()

	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
	newBCN.dataHasArrived = make(chan bool)
	newBCN.HasData = true
	newBCN.DataPointer = data
	newBCN.UTxOManagerIsUpToDate = false

	bc.AllNodesMapMutex.Lock()
	bc.AllNodesMap[_Hash] = newBCN
	bc.AllNodesMapMutex.Unlock()

	bc.MerkleRootToBlockHashMapMutex.Lock()
	bc.MerkleRootToBlockHashMap[block0.MerkleRoot] = _Hash
	bc.MerkleRootToBlockHashMapMutex.Unlock()

	newBCN.PrevBlockChainNode = prevBlock

	bc.Miner.DebugPrint(fmt.Sprintf("added to main chain internally \n"))

	bc.utxoChan <- newBCN

}

// AddData joins the incoming data to the actual structures
func (bc *BlockChain) AddData(data []byte) {

	tb := DeserializeTransactionBlockGroup(data[:], bc)

	merkle := tb.merkleTree.GetMerkleRoot()

	bc.MerkleRootToBlockHashMapMutex.Lock()
	Hash, ok0 := bc.MerkleRootToBlockHashMap[merkle]
	bc.MerkleRootToBlockHashMapMutex.Unlock()

	if !ok0 {

		bc.DanglingDataMutex.RLock()
		_, ok := bc.DanglingData[merkle]
		bc.DanglingDataMutex.RUnlock()

		if ok {
			bc.Miner.DebugPrint("data already known, but still dangling\n")
			return
		}

		bc.DanglingDataMutex.Lock()
		bc.DanglingData[merkle] = tb
		bc.DanglingDataMutex.Unlock()

		bc.Miner.DebugPrint(fmt.Sprintf("added data as dangling, merkle = %x", merkle))
		return
	}

	bc.AllNodesMapMutex.Lock()
	val, ok := bc.AllNodesMap[Hash]
	bc.AllNodesMapMutex.Unlock()

	if !ok {
		bc.Miner.DebugPrint("Merkleroot known, but hashblock unknown, should not happen, root:%x hash:%x\n")
		return
	}

	val.generalMutex.Lock()

	if val.HasData {
		bc.Miner.DebugPrint("received data already added to structure, send confirmation not good")
		val.generalMutex.Unlock()
		return
	}

	val.DataPointer = tb
	val.HasData = true

	val.generalMutex.Unlock()

	bc.Miner.DebugPrint(fmt.Sprintf("added data to struct, hash block %x  sending signal\n", Hash))
	val.dataHasArrived <- true

}

// BlockChainGenesis creates first miner, mines first block and initiates the blockchain
func BlockChainGenesis(difficulty uint32, broadcaster *Broadcaster) *Miner {

	bl := new(BlockChainNode)
	bl.PrevBlockChainNode = bl
	bl.dataHasArrived = make(chan bool)

	bc := new(BlockChain)
	bc.AllNodesMap = make(map[[32]byte]*BlockChainNode)
	bc.OtherHeadNodes = make(map[[32]byte]*BlockChainNode)
	bc.OrphanBlockChains = make([]*BlockChain, 0)
	bc.IsOrphan = false
	bc.Root = bl

	bc.utxoChan = make(chan *BlockChainNode)

	genesisMiner := CreateGenesisMiner("genesis", broadcaster, bc)
	bc.Miner = genesisMiner
	gen := generateGenesis(difficulty, genesisMiner)
	bl.Block = gen
	bl.Hash = bl.Block.Hash()

	bc.Head = bl
	bc.Miner.Debug = false
	bc.AllNodesMap[bl.Hash] = bl
	bc.DanglingData = make(map[[32]byte]*TransactionBlockGroup)
	bc.MerkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	bl.HasData = false
	bl.UTxOManagerIsUpToDate = true
	bl.UTxOManagerPointer = InitializeUTxOMananger(genesisMiner)

	bl.dataHasArrived = make(chan bool)

	go bc.utxoUpdater()

	return genesisMiner
}

// VerifyHash checks to blockchain from head to root
func (bc *BlockChain) VerifyHash() bool {
	BlockChainNode := bc.Head

	for BlockChainNode.Block.BlockCount != 0 {
		if !BlockChainNode.Block.Verify() {
			return false
		}
		if BlockChainNode.Block.PrevHash != BlockChainNode.PrevBlockChainNode.Block.Hash() {
			return false
		}
		BlockChainNode = BlockChainNode.PrevBlockChainNode
	}
	return true
}

// Print print
func (bc *BlockChain) Print() {

	bc.BlockChainMutex.RLock()
	BlockChainNode := bc.Head
	bc.BlockChainMutex.RUnlock()

	BlockChainNode.generalMutex.RLock()

	for {
		k := BlockChainNode

		BlockChainNode.Block.print()
		if BlockChainNode.HasData {
			fmt.Printf("merkleroot from data: %x", BlockChainNode.DataPointer.merkleTree.GetMerkleRoot())
		} else {
			fmt.Printf("this bcn has no data")
		}

		BlockChainNode = BlockChainNode.PrevBlockChainNode

		k.generalMutex.RUnlock()

		if BlockChainNode == nil || BlockChainNode.Block.BlockCount == 0 {
			return
		}

		BlockChainNode.generalMutex.RLock()

	}
}

// PrintHash print
func (bc *BlockChain) PrintHash(n int) {

	bc.BlockChainMutex.RLock()
	BlockChainNode := bc.Head
	bc.BlockChainMutex.RUnlock()

	BlockChainNode.generalMutex.Lock()

	if n == 0 {
		for BlockChainNode != bc.Root {
			k := BlockChainNode
			fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())

			BlockChainNode = BlockChainNode.PrevBlockChainNode

			k.generalMutex.Unlock()

			if BlockChainNode == nil {

				return
			}
			BlockChainNode.generalMutex.Lock()

		}
		BlockChainNode.generalMutex.Unlock()
		return
	}

	for i := 0; i < n; i++ {
		k := BlockChainNode.generalMutex.Unlock

		fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())
		BlockChainNode = BlockChainNode.PrevBlockChainNode

		k()

		if BlockChainNode == nil || BlockChainNode.Block.BlockCount == 0 {
			return
		}

		BlockChainNode.generalMutex.Lock()

	}
	BlockChainNode.generalMutex.Unlock()

}

// SerializeBlockChain make byte array with complete blockchain
func (bc *BlockChain) SerializeBlockChain() [][BlockSize]byte {
	current := bc.Head
	ret := make([][BlockSize]byte, current.Block.BlockCount+1)
	for {
		ret[current.Block.BlockCount] = current.Block.SerializeBlock()
		if current.Block.BlockCount != 0 {
			current = current.PrevBlockChainNode
		} else {
			break
		}
	}
	return ret
}

//DeserializeBlockChain get blockchain from byte array
func DeserializeBlockChain(bc [][BlockSize]byte) *BlockChain {
	blockChain := new(BlockChain)
	blockChain.AllNodesMap = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanBlockChains = make([]*BlockChain, 0)
	blockChain.DanglingData = make(map[[32]byte]*TransactionBlockGroup)
	blockChain.MerkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	blockChain.IsOrphan = false
	blockChain.utxoChan = make(chan *BlockChainNode)

	bcn := new(BlockChainNode)
	bcn.dataHasArrived = make(chan bool)
	bcn.Block = GenesisBlock
	bcn.Hash = GenesisBlock.Hash()
	bcn.PrevBlockChainNode = bcn

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.AllNodesMap[bcn.Hash] = bcn
	blockChain.OtherHeadNodes[bcn.Hash] = bcn

	bcn.UTxOManagerIsUpToDate = false
	bcn.UTxOManagerPointer = nil

	bcn.HasData = false

	go blockChain.utxoUpdater()

	//first block

	for _, bl := range bc[:] {
		blockChain.addBlockChainNode(DeserializeBlock(bl))
	}

	return blockChain
}

// InitializeOrphanBlockChain generates pointer to new bc
func (bc *BlockChain) InitializeOrphanBlockChain(bl *Block) *BlockChain {

	//bc.BlockChainMutex.Lock()
	//defer bc.BlockChainMutex.Unlock()

	blockChain := new(BlockChain)
	blockChain.AllNodesMap = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanBlockChains = nil
	blockChain.IsOrphan = true
	blockChain.DanglingData = make(map[[32]byte]*TransactionBlockGroup)
	blockChain.MerkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	bcn := new(BlockChainNode)

	bcn.dataHasArrived = make(chan bool)

	bcn.Block = bl
	bcn.Hash = bl.Hash()
	bcn.PrevBlockChainNode = nil

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.AllNodesMap[bcn.Hash] = bcn
	blockChain.OtherHeadNodes[bcn.Hash] = bcn

	blockChain.Miner = bc.Miner

	return blockChain
}

// utxoUpdater makes sure one thread at a time is updating the utxo manager
func (bc *BlockChain) utxoUpdater() {
	for {
		bcn := <-bc.utxoChan
		bc.verifyAndBuildDown(bcn)
	}
}

// verifyAndBuildDown request data if necesarry, builds utxo manager from bottum up from last know good state, redirects the upwards links an stores savepoints
//
func (bc *BlockChain) verifyAndBuildDown(bcn *BlockChainNode) {

	bcn.generalMutex.RLock()
	_prevBCN := bcn.PrevBlockChainNode
	_prevHash := bcn.Block.PrevHash
	bcnUTxOManagerIsUpToDate := bcn.UTxOManagerIsUpToDate
	bcnHasData := bcn.HasData
	_newHeight := bcn.Block.BlockCount
	bcn.generalMutex.RUnlock()

	bc.BlockChainMutex.RLock()
	bcIsOrphan := bc.IsOrphan
	bcMiner := bc.Miner
	bc.BlockChainMutex.RUnlock()

	_prevBCN.generalMutex.Lock()
	_prevBCN.NextBlockChainNode = bcn // to restore ability to crawl back up
	prevUTxOUptodate := bcn.PrevBlockChainNode.UTxOManagerIsUpToDate
	prevblockcount := _prevBCN.Block.BlockCount
	_prevBCN.generalMutex.Unlock()

	if bcIsOrphan {
		bc.Miner.DebugPrint("orphans cannot build utxo, returning\n")
		return
	}

	if bcnUTxOManagerIsUpToDate {
		bcMiner.DebugPrint("txo manager already up to date\n")
		return
	}

	var waitingfordata = false
	if !bcnHasData {

		bc.DanglingDataMutex.Lock()
		d, ok := bc.DanglingData[bcn.Block.MerkleRoot]
		bc.DanglingDataMutex.Unlock()

		if !ok {
			var req [32]byte

			bcn.generalMutex.RLock()
			copy(req[0:32], bcn.Hash[0:32])
			bcn.generalMutex.RUnlock()

			go bc.Miner.Request(RequestType["TransactionBlockGroupFromHash"], bc.Miner.Wallet.PublicKey, req[0:32])
			waitingfordata = true
			bcMiner.DebugPrint(fmt.Sprintf("Requesting transactiondata for block %d: %x \n", _newHeight, req[0:32]))
		} else {
			bcMiner.DebugPrint(fmt.Sprintf("found data dangling for block %d: %x \n", bcn.Block.BlockCount, bcn.Hash[:]))
			bc.MerkleRootToBlockHashMapMutex.Lock()
			delete(bc.MerkleRootToBlockHashMap, bcn.Block.MerkleRoot)
			bc.MerkleRootToBlockHashMapMutex.Unlock()

			bcn.generalMutex.Lock()
			bcn.DataPointer = d
			bcn.HasData = true
			bcn.generalMutex.Unlock()
		}

	}

	if !prevUTxOUptodate {
		bc.Miner.DebugPrint(fmt.Sprintf("also updating previous block %d \n", prevblockcount))
		//make sure all locks are released
		bc.verifyAndBuildDown(_prevBCN)

	} //by the time this ends, all previous blocks are updated

	if waitingfordata {
		<-bcn.dataHasArrived
		bc.Miner.DebugPrint(fmt.Sprintf("Requesting transactiondata for block %d arrived ! \n", _newHeight))
	}

	bcn.generalMutex.Lock()
	bcnTbs := bcn.DataPointer.TransactionBlockStructs
	bcn.generalMutex.Unlock()

	_prevBCN.generalMutex.RLock()
	prevUTXO := _prevBCN.UTxOManagerPointer
	_prevBCN.generalMutex.RUnlock()

	goodTransactions := prevUTXO.VerifyTransactionBlockRefs(bcn.DataPointer)

	goodSignatures := true
	for _, tb := range bcnTbs {
		if !tb.VerifyInputSignatures() {
			goodSignatures = false
			break
		}
	}

	bcn.generalMutex.Lock()
	if !goodTransactions {
		bcn.Badblock = true
		bcMiner.DebugPrint("found bad block, transactions not matchin\n")
	}

	if !goodSignatures { // todo check previous blocks!
		bcn.Badblock = true
		bcMiner.DebugPrint("False signatures, not continuing\n")
	}

	bcn.VerifiedWithUtxoManager = true
	bcn.generalMutex.Unlock()

	bc.BlockChainMutex.RLock()
	bcheight := bc.Height
	bc.BlockChainMutex.RUnlock()

	if _newHeight > bcheight {

		bc.OtherHeadNodesMutex.Lock()
		_, ok3 := bc.OtherHeadNodes[_prevHash]
		if ok3 {
			bc.OtherHeadNodes[bc.Head.Hash] = bc.Head
			delete(bc.OtherHeadNodes, bcn.PrevBlockChainNode.Hash)
		}
		bc.OtherHeadNodesMutex.Unlock()

		bc.BlockChainMutex.Lock()
		bc.Head = bcn
		bc.Height = _newHeight
		bc.BlockChainMutex.Unlock()

		bcMiner.GeneralMutex.Lock()
		if bcMiner.isMining {
			bcMiner.interrupt <- true
		}
		bcMiner.GeneralMutex.Unlock()

		bcMiner.DebugPrint("Updated the head after verification")
	}

	keepCopy := _newHeight%5 == 1

	succes := prevUTXO.UpdateWithNextBlockChainNode(bcn, keepCopy)
	if !succes {
		log.Fatal("updating utxoManager Failed, should not happen!\n")
		return
	}
	bcMiner.DebugPrint(fmt.Sprintf("Utxomanager is now at %d, kept copy = %t\n", bcn.Block.BlockCount, keepCopy))

}
