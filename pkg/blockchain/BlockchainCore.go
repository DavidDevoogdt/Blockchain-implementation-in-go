package davidcoin

import (
	"fmt"
	"log"
	"sync"

	"github.com/sasha-s/go-deadlock"
)

// BlockChain keeps the general infromation of all the incoming blocks.
// The most inportant functions related to the blockchain are addBlockChainNode and verifyAndBuildDown
// addBlockChainNode adds a block based on its hash and does internal bookkeeping of the tree. If the block does not fit in the tree, a seperate orphanblockchain is created
// The previous blocks are requested if needed. verifyAndBuildDown updates the the unspent transaction output mananger. Here it is checked whether the block and its data is vallid
type BlockChain struct {
	blockChainMutex   deadlock.RWMutex
	head              *BlockChainNode
	Root              *BlockChainNode
	orphanBlockChains []*BlockChain
	isOrphan          bool
	Height            uint32

	utxoChan         chan *BlockChainNode
	allNodesMap      map[[32]byte]*BlockChainNode
	allNodesMapMutex sync.Mutex

	otherHeadNodes      map[[32]byte]*BlockChainNode
	otherHeadNodesMutex sync.Mutex

	danglingData      map[[32]byte]*transactionBlockGroup // indexed by merkle root, not hash
	danglingDataMutex sync.RWMutex

	merkleRootToBlockHashMap      map[[32]byte][32]byte
	merkleRootToBlockHashMapMutex sync.RWMutex

	Miner *Miner
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	generalMutex           deadlock.RWMutex
	previousBlockChainNode *BlockChainNode
	nextBlockChainNode     *BlockChainNode
	Block                  *block
	Hash                   [32]byte

	hasData     bool
	dataPointer *transactionBlockGroup

	uTxOManagerPointer      *uTxOManager
	uTxOManagerIsUpToDate   bool
	verifiedWithUtxoManager bool
	badblock                bool // true if verified bad

	dataHasArrived chan bool

	knownToTransactionPool bool
}

//#########core

// return bool specifies whether miner should stop immediately
// has no nested locks
func (bc *BlockChain) addBlockChainNode(block0 *block) {

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

	bc.blockChainMutex.RLock()
	bcHead := bc.head
	bcIsOrphan := bc.isOrphan
	bcMiner := bc.Miner
	bc.blockChainMutex.RUnlock()

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash
	_Merkle := block0.MerkleRoot

	bc.allNodesMapMutex.Lock()
	_, nok := bc.allNodesMap[block0.Hash()]
	bc.allNodesMapMutex.Unlock()

	if nok {
		bc.Miner.DebugPrint(fmt.Sprintf("already in chain\n"))
		return
	}

	bc.merkleRootToBlockHashMapMutex.Lock()
	bc.merkleRootToBlockHashMap[_Merkle] = _Hash
	bc.merkleRootToBlockHashMapMutex.Unlock()

	bc.allNodesMapMutex.Lock()
	prevBlock, ok := bc.allNodesMap[_PrevHash]
	bc.allNodesMapMutex.Unlock()

	if !ok {

		for _, obc := range bc.orphanBlockChains[:] {
			obc.blockChainMutex.Lock()
			defer obc.blockChainMutex.Unlock()

			if obc.HasBlock(_PrevHash) {
				bcMiner.DebugPrint(fmt.Sprintf("block added to orphaned chain \n"))

				Endfunctions = append(Endfunctions, func() {
					obc.addBlockChainNode(block0)
				})
				return
			}
		}

		// check whether hash is prevhash of orphan chain root
		for _, obc := range bc.orphanBlockChains[:] {

			if obc.Root.Block.PrevHash == _Hash {
				newBCN := new(BlockChainNode)
				newBCN.Block = block0
				newBCN.Hash = _Hash
				newBCN.dataHasArrived = make(chan bool)
				obc.Root.previousBlockChainNode = newBCN
				newBCN.nextBlockChainNode = obc.Root
				obc.Root = newBCN

				bc.allNodesMapMutex.Lock()
				obc.allNodesMap[_Hash] = newBCN
				bc.allNodesMapMutex.Unlock()

				bcMiner.DebugPrint(fmt.Sprintf("setting new root for orphaned chain \n"))

				go bcMiner.requestBlockFromHash(block0.PrevHash)

				return
			}
		}

		bcMiner.DebugPrint(fmt.Sprintf("prev Block not in history, requesting %.10x\n", _PrevHash))
		bcMiner.DebugPrint(fmt.Sprintf("creating new orphaned chain \n"))

		bc.blockChainMutex.Lock()
		bc.orphanBlockChains = append(bc.orphanBlockChains, bc.initializeOrphanBlockChain(block0))
		bc.blockChainMutex.Unlock()

		go bc.Miner.requestBlockFromHash(block0.PrevHash)

		return

	}

	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
	newBCN.dataHasArrived = make(chan bool)
	newBCN.hasData = false
	newBCN.uTxOManagerIsUpToDate = false

	bc.allNodesMapMutex.Lock()
	bc.allNodesMap[_Hash] = newBCN
	bc.allNodesMapMutex.Unlock()

	prevBlock.generalMutex.Lock()
	prevBlock.nextBlockChainNode = newBCN
	prevBlock.generalMutex.Unlock()

	newBCN.previousBlockChainNode = prevBlock

	// check whether hash is contained as prehash in orphaned chains
	for i, obcR := range bc.orphanBlockChains {

		obc := obcR

		obc.Root.generalMutex.Lock()

		if _Hash == obc.Root.Block.PrevHash {
			//defer obc.BlockChainMutex.Unlock()

			obc.Root.previousBlockChainNode = newBCN
			obcHead := obc.head
			obc.Root.generalMutex.Unlock()

			bcMiner.DebugPrint(fmt.Sprintf("joining orphaned chain to main chain!\n"))
			// delete prev node from head if that were the case

			bc.otherHeadNodesMutex.Lock()
			delete(bc.otherHeadNodes, _PrevHash)
			for k, v := range obc.otherHeadNodes { //merge hashmaps
				bc.otherHeadNodes[k] = v
			}
			bc.otherHeadNodesMutex.Unlock()

			bc.allNodesMapMutex.Lock()
			for k, v := range obc.allNodesMap {
				bc.allNodesMap[k] = v
			}
			bc.allNodesMapMutex.Unlock()

			// put orphaned blockchain to last element and remove it
			l := len(bc.orphanBlockChains)

			bc.blockChainMutex.Lock()
			bc.orphanBlockChains[i] = bc.orphanBlockChains[l-1]
			bc.orphanBlockChains = bc.orphanBlockChains[:l-1]
			bc.blockChainMutex.Unlock()

			orphanHead := obc.head

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

	bc.otherHeadNodesMutex.Lock()
	_, ok = bc.otherHeadNodes[_PrevHash]
	bc.otherHeadNodesMutex.Unlock()

	if ok {
		bcMiner.DebugPrint(fmt.Sprintf("added to head chain \n"))

		bc.otherHeadNodesMutex.Lock()
		delete(bc.otherHeadNodes, _PrevHash)
		bc.otherHeadNodes[_Hash] = newBCN
		bc.otherHeadNodesMutex.Unlock()

	} else {

		bc.otherHeadNodesMutex.Lock()
		bc.otherHeadNodes[newBCN.Hash] = newBCN
		bc.otherHeadNodesMutex.Unlock()
		bcMiner.DebugPrint(fmt.Sprintf("new fork created \n"))
	}

	bcHead.generalMutex.Lock()
	hlen := bcHead.Block.BlockCount

	if hlen < newBCN.Block.BlockCount {
		if bcIsOrphan {
			bc.head = newBCN
		} else {
			Endfunctions = append(Endfunctions, func() {
				go func() { bc.utxoChan <- newBCN }()
			})

		}

	}

	bcHead.generalMutex.Unlock()

}

//addInternal is used to add mined block and data at once
func (bc *BlockChain) addInternal(block0 *block, data *transactionBlockGroup) {
	// todo add checks

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash

	bc.allNodesMapMutex.Lock()
	prevBlock := bc.allNodesMap[_PrevHash]
	bc.allNodesMapMutex.Unlock()

	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
	newBCN.dataHasArrived = make(chan bool)
	newBCN.hasData = true
	newBCN.dataPointer = data
	newBCN.uTxOManagerIsUpToDate = false

	bc.allNodesMapMutex.Lock()
	bc.allNodesMap[_Hash] = newBCN
	bc.allNodesMapMutex.Unlock()

	bc.merkleRootToBlockHashMapMutex.Lock()
	bc.merkleRootToBlockHashMap[block0.MerkleRoot] = _Hash
	bc.merkleRootToBlockHashMapMutex.Unlock()

	newBCN.previousBlockChainNode = prevBlock

	bc.Miner.DebugPrint(fmt.Sprintf("added to main chain internally \n"))

	bc.utxoChan <- newBCN

}

// addData joins the incoming data to the actual structures
func (bc *BlockChain) addData(data []byte) {

	tb := deserializeTransactionBlockGroup(data[:], bc)

	merkle := tb.merkleTree.GetMerkleRoot()

	bc.merkleRootToBlockHashMapMutex.Lock()
	Hash, ok0 := bc.merkleRootToBlockHashMap[merkle]
	bc.merkleRootToBlockHashMapMutex.Unlock()

	if !ok0 {

		bc.danglingDataMutex.RLock()
		_, ok := bc.danglingData[merkle]
		bc.danglingDataMutex.RUnlock()

		if ok {
			bc.Miner.DebugPrint("data already known, but still dangling\n")
			return
		}

		bc.danglingDataMutex.Lock()
		bc.danglingData[merkle] = tb
		bc.danglingDataMutex.Unlock()

		bc.Miner.DebugPrint(fmt.Sprintf("added data as dangling, merkle = %x", merkle))
		return
	}

	bc.allNodesMapMutex.Lock()
	val, ok := bc.allNodesMap[Hash]
	bc.allNodesMapMutex.Unlock()

	if !ok {
		bc.Miner.DebugPrint("Merkleroot known, but hashblock unknown, should not happen, root:%x hash:%x\n")
		return
	}

	val.generalMutex.Lock()

	if val.hasData {
		bc.Miner.DebugPrint("received data already added to structure, send confirmation not good\n")
		val.generalMutex.Unlock()
		return
	}

	val.dataPointer = tb
	val.hasData = true

	val.generalMutex.Unlock()

	bc.Miner.DebugPrint(fmt.Sprintf("added data to struct, hash block %x  sending signal\n", Hash))
	val.dataHasArrived <- true

}

// utxoUpdater makes sure one thread at a time is updating the utxo manager
func (bc *BlockChain) utxoUpdater() {
	for {
		bcn := <-bc.utxoChan
		bc.verifyAndBuildDown(bcn)
	}
}

// verifyAndBuildDown request data if necesarry, builds utxo manager from bottum up from last know good state, redirects the upwards links an stores savepoints
func (bc *BlockChain) verifyAndBuildDown(bcn *BlockChainNode) {

	bcn.generalMutex.RLock()
	_prevBCN := bcn.previousBlockChainNode
	_prevHash := bcn.Block.PrevHash
	bcnUTxOManagerIsUpToDate := bcn.uTxOManagerIsUpToDate
	bcnhasData := bcn.hasData
	_newHeight := bcn.Block.BlockCount
	bcn.generalMutex.RUnlock()

	bc.blockChainMutex.RLock()
	bcIsOrphan := bc.isOrphan
	bcMiner := bc.Miner
	bc.blockChainMutex.RUnlock()

	_prevBCN.generalMutex.Lock()
	_prevBCN.nextBlockChainNode = bcn // to restore ability to crawl back up
	prevUTxOUptodate := bcn.previousBlockChainNode.uTxOManagerIsUpToDate
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
	if !bcnhasData {

		bc.danglingDataMutex.Lock()
		d, ok := bc.danglingData[bcn.Block.MerkleRoot]
		bc.danglingDataMutex.Unlock()

		if !ok {
			var req [32]byte

			bcn.generalMutex.RLock()
			copy(req[0:32], bcn.Hash[0:32])
			bcn.generalMutex.RUnlock()

			go bc.Miner.request(requestType["TransactionBlockGroupFromHash"], bc.Miner.Wallet.PublicKey, req[0:32])
			waitingfordata = true
			bcMiner.DebugPrint(fmt.Sprintf("Requesting transactiondata for block %d: %x \n", _newHeight, req[0:32]))
		} else {
			bcMiner.DebugPrint(fmt.Sprintf("found data dangling for block %d: %x \n", bcn.Block.BlockCount, bcn.Hash[:]))
			bc.merkleRootToBlockHashMapMutex.Lock()
			delete(bc.merkleRootToBlockHashMap, bcn.Block.MerkleRoot)
			bc.merkleRootToBlockHashMapMutex.Unlock()

			bcn.generalMutex.Lock()
			bcn.dataPointer = d
			bcn.hasData = true
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
	bcnTbs := bcn.dataPointer
	bcn.generalMutex.Unlock()

	_prevBCN.generalMutex.RLock()
	prevUTXO := _prevBCN.uTxOManagerPointer
	_prevBCN.generalMutex.RUnlock()

	goodTransactions := prevUTXO.verifyTransactionBlockGroupRefs(bcnTbs)
	goodSignatures := bcnTbs.VerifyExceptUTXO()

	bcn.generalMutex.Lock()
	if !goodTransactions {
		bcn.badblock = true
		bcMiner.DebugPrint("found bad block, transactions not matchin\n")
	}

	if !goodSignatures { // todo check previous blocks!
		bcn.badblock = true
		bcMiner.DebugPrint("False signatures, not continuing\n")
	}

	bcn.verifiedWithUtxoManager = true
	bcn.generalMutex.Unlock()

	bc.blockChainMutex.RLock()
	bcheight := bc.Height
	bc.blockChainMutex.RUnlock()

	if _newHeight > bcheight {

		bc.otherHeadNodesMutex.Lock()
		_, ok3 := bc.otherHeadNodes[_prevHash]
		if ok3 {
			bc.otherHeadNodes[bc.head.Hash] = bc.head
			delete(bc.otherHeadNodes, bcn.previousBlockChainNode.Hash)
		}
		bc.otherHeadNodesMutex.Unlock()

		bc.blockChainMutex.Lock()
		bc.head = bcn
		bc.Height = _newHeight
		bc.blockChainMutex.Unlock()

		bcMiner.generalMutex.Lock()
		if bcMiner.isMining {
			bcMiner.interrupt <- true
		}
		bcMiner.generalMutex.Unlock()

		bcMiner.DebugPrint("Updated the head after verification\n")
	}

	keepCopy := _newHeight%5 == 1

	succes := prevUTXO.updateWithNextBlockChainNode(bcn, keepCopy)
	if !succes {
		log.Fatal(fmt.Sprintf("updating utxoManager Failed, should not happen! %s\n", bc.Miner.Name))
		return
	}

	bcMiner.DebugPrint(fmt.Sprintf("Utxomanager is now at %d, kept copy = %t\n", bcn.Block.BlockCount, keepCopy))
	bc.Miner.Wallet.updateWithBlock(bcn)
	bc.Miner.tp.updateWithBlockchainNode(bcn)

}

//###############initalizationcode

// SerializeBlockChain make byte array with complete blockchain
func (bc *BlockChain) SerializeBlockChain() [][blockSize]byte {
	current := bc.head
	ret := make([][blockSize]byte, current.Block.BlockCount+1)
	for {
		ret[current.Block.BlockCount] = current.Block.serializeBlock()
		if current.Block.BlockCount != 0 {
			current = current.previousBlockChainNode
		} else {
			break
		}
	}
	return ret
}

//DeserializeBlockChain get blockchain from byte array
func DeserializeBlockChain(bc [][blockSize]byte) *BlockChain {
	blockChain := new(BlockChain)
	blockChain.allNodesMap = make(map[[32]byte]*BlockChainNode)
	blockChain.otherHeadNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.orphanBlockChains = make([]*BlockChain, 0)
	blockChain.danglingData = make(map[[32]byte]*transactionBlockGroup)
	blockChain.merkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	blockChain.isOrphan = false
	blockChain.utxoChan = make(chan *BlockChainNode)

	bcn := new(BlockChainNode)
	bcn.dataHasArrived = make(chan bool)
	bcn.Block = genesisBlock
	bcn.Hash = genesisBlock.Hash()
	bcn.previousBlockChainNode = bcn

	blockChain.head = bcn
	blockChain.Root = bcn
	blockChain.allNodesMap[bcn.Hash] = bcn
	blockChain.otherHeadNodes[bcn.Hash] = bcn

	bcn.uTxOManagerIsUpToDate = false
	bcn.uTxOManagerPointer = nil

	bcn.hasData = false

	//first block

	for _, bl := range bc[:] {
		blockChain.addBlockChainNode(deserializeBlock(bl))
	}

	return blockChain
}

// initializeOrphanBlockChain generates pointer to new bc
func (bc *BlockChain) initializeOrphanBlockChain(bl *block) *BlockChain {

	//bc.BlockChainMutex.Lock()
	//defer bc.BlockChainMutex.Unlock()

	blockChain := new(BlockChain)
	blockChain.allNodesMap = make(map[[32]byte]*BlockChainNode)
	blockChain.otherHeadNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.orphanBlockChains = nil
	blockChain.isOrphan = true
	blockChain.danglingData = make(map[[32]byte]*transactionBlockGroup)
	blockChain.merkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	bcn := new(BlockChainNode)

	bcn.dataHasArrived = make(chan bool)

	bcn.Block = bl
	bcn.Hash = bl.Hash()
	bcn.previousBlockChainNode = nil

	blockChain.head = bcn
	blockChain.Root = bcn
	blockChain.allNodesMap[bcn.Hash] = bcn
	blockChain.otherHeadNodes[bcn.Hash] = bcn

	blockChain.Miner = bc.Miner

	return blockChain
}

//

//####### helpfunctions

// HasBlock determines whether block is in chain
func (bc *BlockChain) HasBlock(hash [32]byte) bool {
	bc.allNodesMapMutex.Lock()
	_, ok := bc.allNodesMap[hash]
	bc.allNodesMapMutex.Unlock()
	return ok
}

// GetBlockChainNodeAtHeight returns block of main chain at specific height
func (bc *BlockChain) GetBlockChainNodeAtHeight(height uint32) *BlockChainNode {
	//bc.BlockChainMutex.RLock()
	//defer bc.BlockChainMutex.RUnlock()

	current := bc.head

	max := current.Block.BlockCount

	if height > max {
		return nil
	}

	for i := uint32(0); i < max-height; i++ {

		current.generalMutex.RLock()
		defer current.generalMutex.RUnlock()
		current = current.previousBlockChainNode
	}

	//bc.Miner.DebugPrint(("requested %d, got %d", height, current.Block.BlockCount)

	return current
}

// GetBlockChainNodeAtHash fetches block
func (bc *BlockChain) GetBlockChainNodeAtHash(hash [32]byte) *BlockChainNode {
	bc.allNodesMapMutex.Lock()
	val, ok := bc.allNodesMap[hash]
	bc.allNodesMapMutex.Unlock()

	if ok {
		return val
	}

	bc.blockChainMutex.Lock()
	for _, obc := range bc.orphanBlockChains {
		bc.blockChainMutex.Unlock()
		val := obc.GetBlockChainNodeAtHash(hash)
		if val != nil {
			return val
		}
		bc.blockChainMutex.Lock()
	}
	bc.blockChainMutex.Unlock()

	return nil
}

// getTransactionOutput turns reference into locally saved version err: 0->ok 1->block unknown 2->no data 3->data does not exist
func (bc *BlockChain) getTransactionOutput(tr *transactionRef) (*transactionOutput, uint8) {

	bc.allNodesMapMutex.Lock()
	bl, ok := bc.allNodesMap[tr.BlockHash]
	bc.allNodesMapMutex.Unlock()

	if !ok {
		return nil, uint8(1)
	}

	bl.generalMutex.RLock()
	if bl.hasData == false {
		bl.generalMutex.RUnlock()
		return nil, uint8(2)
	}
	dp := bl.dataPointer
	bl.generalMutex.RUnlock()

	if dp.size <= tr.TransactionBlockNumber {
		return nil, uint8(4)
	}

	outpBl := dp.TransactionBlockStructs[tr.TransactionBlockNumber]

	if outpBl.OutputNumber <= tr.OutputNumber {
		return nil, uint8(4)
	}

	return outpBl.OutputList[tr.OutputNumber], uint8(0)
}

// VerifyHash checks to blockchain from head to root
func (bc *BlockChain) VerifyHash() bool {
	BlockChainNode := bc.head

	for BlockChainNode.Block.BlockCount != 0 {
		if !BlockChainNode.Block.Verify() {
			return false
		}
		if BlockChainNode.Block.PrevHash != BlockChainNode.previousBlockChainNode.Block.Hash() {
			return false
		}
		BlockChainNode = BlockChainNode.previousBlockChainNode
	}
	return true
}

// Print print
func (bc *BlockChain) Print() {

	bc.blockChainMutex.RLock()
	BlockChainNode := bc.head
	bc.blockChainMutex.RUnlock()

	BlockChainNode.generalMutex.RLock()

	for {
		k := BlockChainNode

		BlockChainNode.Block.print()
		if BlockChainNode.hasData {
			fmt.Printf("merkleroot from data: %x", BlockChainNode.dataPointer.merkleTree.GetMerkleRoot())
		} else {
			fmt.Printf("this bcn has no data")
		}

		BlockChainNode = BlockChainNode.previousBlockChainNode

		k.generalMutex.RUnlock()

		if BlockChainNode == nil || BlockChainNode.Block.BlockCount == 0 {
			return
		}

		BlockChainNode.generalMutex.RLock()

	}
}

// PrintHash print
func (bc *BlockChain) PrintHash(n int) {

	bc.blockChainMutex.RLock()
	BlockChainNode := bc.head
	bc.blockChainMutex.RUnlock()

	BlockChainNode.generalMutex.Lock()

	if n == 0 {
		for BlockChainNode != bc.Root {
			k := BlockChainNode
			fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())

			BlockChainNode = BlockChainNode.previousBlockChainNode

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
		BlockChainNode = BlockChainNode.previousBlockChainNode

		k()

		if BlockChainNode == nil || BlockChainNode.Block.BlockCount == 0 {
			return
		}

		BlockChainNode.generalMutex.Lock()

	}
	BlockChainNode.generalMutex.Unlock()

}

//GetHead returns the thead of the blockchain
func (bc *BlockChain) GetHead() *BlockChainNode {

	bc.blockChainMutex.RLock()
	head := bc.head
	bc.blockChainMutex.RUnlock()

	return head
}

//GetBlockNum returns the block numbers of the referenced blockchainNode
func (bcn *BlockChainNode) GetBlockNum() uint32 {
	bcn.generalMutex.RLock()
	defer bcn.generalMutex.RUnlock()

	return bcn.Block.BlockCount
}
