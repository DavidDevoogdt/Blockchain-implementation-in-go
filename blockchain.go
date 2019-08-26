package main

import (
	"fmt"
	"log"
	"sync"
)

// BlockChain is keeps track of known heads of tree
type BlockChain struct {
	Head *BlockChainNode
	Root *BlockChainNode

	AllNodesMap      map[[32]byte]*BlockChainNode
	AllNodesMapMutex sync.Mutex

	OtherHeadNodes      map[[32]byte]*BlockChainNode
	OtherHeadNodesMutex sync.Mutex

	Miner             *Miner
	OrphanBlockChains []*BlockChain
	IsOrphan          bool

	DanglingData      map[[32]byte]*TransactionBlockGroup // indexed by merkle root, not hash
	DanglingDataMutex sync.RWMutex

	MerkleRootToBlockHashMap      map[[32]byte][32]byte
	MerkleRootToBlockHashMapMutex sync.RWMutex
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	PrevBlockChainNode *BlockChainNode
	NextBlockChainNode *BlockChainNode
	Block              *Block
	Hash               [32]byte

	HasData     bool
	DataPointer *TransactionBlockGroup

	UTxOManagerPointer    *UTxOManager
	UTxOManagerIsUpToDate bool

	Badblock bool // true if verified bad

	VerifiedWithUtxoManager bool
	dataHasArrived          chan bool

	utxoMutex sync.Mutex
}

// HasBlock determines whether block is in chain
func (bc *BlockChain) HasBlock(hash [32]byte) bool {
	bc.AllNodesMapMutex.Lock()
	_, ok := bc.AllNodesMap[hash]
	bc.AllNodesMapMutex.Unlock()
	return ok
}

// GetBlockChainNodeAtHeight returns block of main chain at specific height
func (bc *BlockChain) GetBlockChainNodeAtHeight(height uint32) *BlockChainNode {
	current := bc.Head
	max := current.Block.BlockCount
	if height > max {
		return nil
	}

	for i := uint32(0); i < max-height; i++ {
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

	for _, obc := range bc.OrphanBlockChains {
		val := obc.GetBlockChainNodeAtHash(hash)
		if val != nil {
			return val
		}
	}

	return nil
}

// GetTransactionOutput turns reference into locally saved version
func (bc *BlockChain) GetTransactionOutput(tr *TransactionRef) *TransactionOutput {
	// todo verifiy this data is present
	return bc.AllNodesMap[tr.BlockHash].DataPointer.TransactionBlockStructs[tr.TransactionBlockNumber].OutputList[tr.OutputNumber]
}

// return bool specifies whether miner should stop immediately
func (bc *BlockChain) addBlockChainNode(block0 *Block) {

	if !block0.Verify() {
		bc.Miner.DebugPrint(fmt.Sprintf("Block not ok, not added\n"))
		return
	}

	bc.AllNodesMapMutex.Lock()
	_, nok := bc.AllNodesMap[block0.Hash()]
	bc.AllNodesMapMutex.Unlock()

	if nok {
		bc.Miner.DebugPrint(fmt.Sprintf("already in chain\n"))
		return
	}

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash
	_Merkle := block0.MerkleRoot

	bc.MerkleRootToBlockHashMapMutex.Lock()
	bc.MerkleRootToBlockHashMap[_Merkle] = _Hash
	bc.MerkleRootToBlockHashMapMutex.Unlock()

	bc.AllNodesMapMutex.Lock()
	prevBlock, ok := bc.AllNodesMap[_PrevHash]
	bc.AllNodesMapMutex.Unlock()

	if !ok {

		for _, obc := range bc.OrphanBlockChains[:] {
			if obc.HasBlock(_PrevHash) {
				bc.Miner.DebugPrint(fmt.Sprintf("block added to orphaned chain \n"))
				obc.addBlockChainNode(block0)
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

				go bc.Miner.RequestBlockFromHash(block0.PrevHash)

				bc.Miner.DebugPrint(fmt.Sprintf("setting new root for orphaned chain \n"))

				return
			}
		}

		bc.Miner.DebugPrint(fmt.Sprintf("prev Block not in history, requesting %.10x\n", _PrevHash))
		bc.Miner.DebugPrint(fmt.Sprintf("creating new orphaned chain \n"))
		bc.OrphanBlockChains = append(bc.OrphanBlockChains, bc.InitializeOrphanBlockChain(block0))

		go bc.Miner.RequestBlockFromHash(block0.PrevHash)

		return

	}
	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
	newBCN.dataHasArrived = make(chan bool, 5)
	newBCN.HasData = false
	newBCN.UTxOManagerIsUpToDate = false

	bc.AllNodesMapMutex.Lock()
	bc.AllNodesMap[_Hash] = newBCN
	bc.AllNodesMapMutex.Unlock()

	newBCN.PrevBlockChainNode = prevBlock
	prevBlock.NextBlockChainNode = newBCN

	// check whether hash is contained as prehash in orphaned chains
	for i, obc := range bc.OrphanBlockChains {
		if _Hash == obc.Root.Block.PrevHash {
			bc.Miner.DebugPrint(fmt.Sprintf("joining orphaned chain to main chain!\n"))
			obc.Root.PrevBlockChainNode = newBCN

			// delete prev node from head if that were the case

			bc.OtherHeadNodesMutex.Lock()
			delete(bc.OtherHeadNodes, _PrevHash)
			//merge hashmaps
			for k, v := range obc.OtherHeadNodes {
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
			if l == 1 {
				bc.OrphanBlockChains = make([]*BlockChain, 0)
			} else {
				bc.OrphanBlockChains[i] = bc.OrphanBlockChains[l-1]
				bc.OrphanBlockChains = bc.OrphanBlockChains[:l-1]
			}

			orphanHead := obc.Head

			if orphanHead.Block.BlockCount > bc.Head.Block.BlockCount {
				bc.Miner.DebugPrint(fmt.Sprintf("orphaned chain is now main chain\n"))
				bc.VerifyAndBuildDown(orphanHead)
				return
			}

			return

		}
	}

	//nothing special, just main chain

	bc.OtherHeadNodesMutex.Lock()
	_, ok = bc.OtherHeadNodes[_PrevHash]
	bc.OtherHeadNodesMutex.Unlock()

	if ok {
		bc.Miner.DebugPrint(fmt.Sprintf("added to head chain \n"))

		bc.OtherHeadNodesMutex.Lock()
		delete(bc.OtherHeadNodes, _PrevHash)
		bc.OtherHeadNodes[_Hash] = newBCN
		bc.OtherHeadNodesMutex.Unlock()

	} else {

		bc.OtherHeadNodesMutex.Lock()
		bc.OtherHeadNodes[newBCN.Hash] = newBCN
		bc.OtherHeadNodesMutex.Unlock()
		bc.Miner.DebugPrint(fmt.Sprintf("new fork created \n"))
	}
	//append to side chain

	if bc.Head.Block.BlockCount < newBCN.Block.BlockCount {
		if bc.IsOrphan {
			bc.Head = newBCN
		} else {
			bc.VerifyAndBuildDown(newBCN)
		}

	}

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
	newBCN.dataHasArrived = make(chan bool, 5)
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
	prevBlock.NextBlockChainNode = newBCN

	bc.Miner.DebugPrint(fmt.Sprintf("added to main chain internally \n"))

	//bc.Head = newBCN
	bc.VerifyAndBuildDown(newBCN)

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

	if val.HasData {
		bc.Miner.DebugPrint("received data already added to structure, send confirmation not good")
		return
	}

	val.DataPointer = tb
	val.HasData = true

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
	BlockChainNode := bc.Head
	BlockChainNode.Block.print()
	for BlockChainNode.Block.BlockCount != 0 {
		BlockChainNode = BlockChainNode.PrevBlockChainNode
		BlockChainNode.Block.print()
		if BlockChainNode.HasData {
			fmt.Printf("merkleroot from data: %x", BlockChainNode.DataPointer.merkleTree.GetMerkleRoot())
		} else {
			fmt.Printf("this bcn has no data")
		}

	}
}

// PrintHash print
func (bc *BlockChain) PrintHash(n int) {

	BlockChainNode := bc.Head
	fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())
	if n == 0 {
		for BlockChainNode != bc.Root {

			BlockChainNode = BlockChainNode.PrevBlockChainNode
			if BlockChainNode == nil {
				return
			}
			fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())

		}
		return
	}

	for i := 0; i < n; i++ {
		BlockChainNode = BlockChainNode.PrevBlockChainNode
		if BlockChainNode == nil || BlockChainNode.Block.BlockCount == 0 {
			return
		}
		fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())
	}

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

	//first block

	for _, bl := range bc[:] {
		blockChain.addBlockChainNode(DeserializeBlock(bl))
	}

	return blockChain
}

// InitializeOrphanBlockChain generates pointer to new bc
func (bc *BlockChain) InitializeOrphanBlockChain(bl *Block) *BlockChain {
	blockChain := new(BlockChain)
	blockChain.AllNodesMap = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanBlockChains = nil
	blockChain.IsOrphan = true
	blockChain.DanglingData = make(map[[32]byte]*TransactionBlockGroup)
	blockChain.MerkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	bcn := new(BlockChainNode)

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

// VerifyAndBuildDown request data if necesarry, builds utxo manager from bottum up from last know good state, redirects the upwards links an stores savepoints
func (bc *BlockChain) VerifyAndBuildDown(bcn *BlockChainNode) {

	bcn.utxoMutex.Lock()

	if bc.IsOrphan {
		bc.Miner.DebugPrint("orphans cannot build utxo, returning\n")
		return
	}

	if bcn.UTxOManagerIsUpToDate {
		bc.Miner.DebugPrint("txo manager already up to date\n")
		return
	}

	bcn.PrevBlockChainNode.NextBlockChainNode = bcn // to restore ability to crawl back up

	var waitingfordata = false

	//todo check for dangling data

	if !bcn.HasData {

		bc.DanglingDataMutex.Lock()
		d, ok := bc.DanglingData[bcn.Block.MerkleRoot]
		bc.DanglingDataMutex.Unlock()

		if !ok {
			var req [32]byte
			copy(req[0:32], bcn.Hash[0:32])
			bcn.dataHasArrived = make(chan bool)
			go bc.Miner.Request(RequestType["TransactionBlockGroupFromHash"], bc.Miner.Wallet.PublicKey, req[0:32])
			waitingfordata = true

			bc.Miner.DebugPrint(fmt.Sprintf("Requesting transactiondata for block %d: %x \n", bcn.Block.BlockCount, bcn.Hash[:]))
		} else {
			bc.Miner.DebugPrint(fmt.Sprintf("found data dangling for block %d: %x \n", bcn.Block.BlockCount, bcn.Hash[:]))
			bc.MerkleRootToBlockHashMapMutex.Lock()
			delete(bc.MerkleRootToBlockHashMap, bcn.Block.MerkleRoot)
			bc.MerkleRootToBlockHashMapMutex.Unlock()

			bcn.DataPointer = d
			bcn.HasData = true
		}

	}

	if !bcn.PrevBlockChainNode.UTxOManagerIsUpToDate {
		bc.Miner.DebugPrint(fmt.Sprintf("also updating previous block %d \n", bcn.PrevBlockChainNode.Block.BlockCount))
		bc.VerifyAndBuildDown(bcn.PrevBlockChainNode)
	} //by the time this ends, all previous blocks are updated

	if waitingfordata {
		<-bcn.dataHasArrived
		bc.Miner.DebugPrint(fmt.Sprintf("Requesting transactiondata for block %d arrived ! \n", bcn.Block.BlockCount))
	}

	goodTransactions := bcn.PrevBlockChainNode.UTxOManagerPointer.VerifyTransactionBlockRefs(bcn.DataPointer)
	if !goodTransactions {
		bcn.Badblock = true
		bc.Miner.DebugPrint("found bad block, transactions not matchin\n")
	}

	goodSignatures := true
	for _, tb := range bcn.DataPointer.TransactionBlockStructs {
		if !tb.VerifyInputSignatures() {
			goodSignatures = false
			return
		}
	}

	if !goodSignatures {
		bcn.Badblock = true
		bc.Miner.DebugPrint("False signatures, not continuing\n")
	}

	bcn.VerifiedWithUtxoManager = true

	//update the head if necessary
	if bcn.Block.BlockCount > bc.Head.Block.BlockCount {

		_PrevHash := bcn.PrevBlockChainNode.Hash

		bc.OtherHeadNodesMutex.Lock()
		_, ok3 := bc.OtherHeadNodes[_PrevHash]
		if ok3 {
			bc.OtherHeadNodes[bc.Head.Hash] = bc.Head
			delete(bc.OtherHeadNodes, bcn.PrevBlockChainNode.Hash)
		}
		bc.OtherHeadNodesMutex.Unlock()

		bc.Head = bcn

		if bc.Miner.isMining {
			bc.Miner.interrupt <- true
		}

		bc.Miner.DebugPrint("Updated the head after verification")
	}

	keepCopy := bcn.Block.BlockCount%5 == 1

	succes := bcn.PrevBlockChainNode.UTxOManagerPointer.UpdateWithNextBlockChainNode(bcn, keepCopy)
	if !succes {
		log.Fatal("updating utxoManager Failed, should not happen!\n")
		return
	}
	bc.Miner.DebugPrint(fmt.Sprintf("Utxomanager is now at %d, kept copy = %t\n", bcn.Block.BlockCount, keepCopy))

	bcn.utxoMutex.Unlock()

}
