package main

import (
	"crypto/sha256"
	"fmt"
	"log"
	"sync"
)

// BlockChain is keeps track of known heads of tree
type BlockChain struct {
	Head *BlockChainNode
	Root *BlockChainNode

	AllNodesMap         map[[32]byte]*BlockChainNode
	AllNodesMapMutex    sync.Mutex
	OtherHeadNodes      map[[32]byte]*BlockChainNode
	OtherHeadNodesMutex sync.Mutex

	Miner             *Miner
	OrphanBlockChains []*BlockChain
	IsOrphan          bool

	DanglingData      map[[32]byte][]byte
	DanglingDataMutex sync.RWMutex
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	PrevBlockChainNode    *BlockChainNode
	NextBlockChainNode    *BlockChainNode
	Block                 *Block
	Hash                  [32]byte
	HasData               bool
	DataPointer           *TransactionBlock
	UTxOManagerPointer    *UTxOManager
	UTxOManagerIsUpToDate bool
	Badblock              bool // true if verified bad
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
	return bc.AllNodesMap[tr.BlockHash].DataPointer.OutputList[tr.Number]
}

// return bool specifies whether miner should stop immediately
func (bc *BlockChain) addBlockChainNode(block0 *Block) bool {
	k := bc.Miner

	var name string
	if k != nil {
		name = fmt.Sprintf("%s", bc.Miner.Name)
	} else {
		name = fmt.Sprintf("unknown")
	}

	if !block0.Verify() {
		bc.Miner.DebugPrint(fmt.Sprintf("-%s----Block not ok, not added\n", name))
		return false
	}

	bc.AllNodesMapMutex.Lock()
	_, nok := bc.AllNodesMap[block0.Hash()]
	bc.AllNodesMapMutex.Unlock()

	if nok {
		bc.Miner.DebugPrint(fmt.Sprintf("%s-----already in chain\n", name))
		return false
	}

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash

	bc.AllNodesMapMutex.Lock()
	prevBlock, ok := bc.AllNodesMap[_PrevHash]
	bc.AllNodesMapMutex.Unlock()

	if !ok {
		//debug<- fmt.Sprintf("OOOOOOOOrphan: got %x with prehash %x", _Hash, _PrevHash)
		// check whether prevhash in orphan chain
		for _, obc := range bc.OrphanBlockChains[:] {
			if obc.HasBlock(_PrevHash) {
				bc.Miner.DebugPrint(fmt.Sprintf("-----block added to orphaned chain \n"))
				obc.addBlockChainNode(block0)
				return false
			}
		}

		// check whether hash is prevhash of orphan chain root
		for _, obc := range bc.OrphanBlockChains[:] {
			if obc.Root.Block.PrevHash == _Hash {
				newBCN := new(BlockChainNode)
				newBCN.Block = block0
				newBCN.Hash = _Hash
				obc.Root.PrevBlockChainNode = newBCN
				newBCN.NextBlockChainNode = obc.Root
				obc.Root = newBCN

				bc.AllNodesMapMutex.Lock()
				obc.AllNodesMap[_Hash] = newBCN
				bc.AllNodesMapMutex.Unlock()

				//go bc.Miner.HashRequestBlock(block0.PrevHash)

				go bc.Miner.RequestBlockFromHash(block0.PrevHash)

				bc.Miner.DebugPrint(fmt.Sprintf("%s-----setting new root for orphaned chain \n", name))
				return false
			}
		}

		bc.Miner.DebugPrint(fmt.Sprintf("%s-----prev Block not in history, requesting %.10x\n", name, _PrevHash))
		bc.Miner.DebugPrint(fmt.Sprintf("%s-----creating new orphaned chain \n", name))
		bc.OrphanBlockChains = append(bc.OrphanBlockChains, bc.InitializeOrphanBlockChain(block0))

		go bc.Miner.RequestBlockFromHash(block0.PrevHash)
		return false

	}
	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
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
			bc.Miner.DebugPrint(fmt.Sprintf("%s-----joining orphaned chain to main chain!\n", name))
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
			bc.OrphanBlockChains[i] = bc.OrphanBlockChains[l-1]
			bc.OrphanBlockChains = bc.OrphanBlockChains[:l-1]

			mainHead := bc.Head
			orphanHead := obc.Head
			if orphanHead.Block.BlockCount > bc.Head.Block.BlockCount {
				bc.OtherHeadNodesMutex.Lock()
				bc.OtherHeadNodes[mainHead.Hash] = mainHead
				bc.OtherHeadNodesMutex.Unlock()
				bc.Head = orphanHead
				bc.Miner.DebugPrint(fmt.Sprintf("%s-----orphaned chain is now main chain\n", name))
				return true
			}

			bc.OtherHeadNodesMutex.Lock()
			bc.OtherHeadNodes[orphanHead.Hash] = orphanHead
			bc.OtherHeadNodesMutex.Unlock()
			return false

		}
	}

	//nothing special, just main chain
	if prevBlock.Hash != bc.Head.Hash {
		bc.OtherHeadNodesMutex.Lock()
		_, ok = bc.OtherHeadNodes[_PrevHash]
		bc.OtherHeadNodesMutex.Unlock()

		if ok { ////non primary chain got longer, check whether new longest
			if bc.Head.Block.BlockCount < block0.BlockCount { /////got new best
				bc.OtherHeadNodesMutex.Lock()
				bc.OtherHeadNodes[bc.Head.Hash] = bc.Head
				delete(bc.OtherHeadNodes, block0.PrevHash)
				bc.OtherHeadNodesMutex.Unlock()

				bc.Head = newBCN
				ptr := bc.Head
				//set new chain for forward linked nodes
				for i := newBCN.Block.BlockCount; i > 0; i++ {
					ptr.PrevBlockChainNode.NextBlockChainNode = ptr
					ptr = ptr.PrevBlockChainNode
				}
				bc.Miner.DebugPrint(fmt.Sprintf("-----block added as new head \n"))
				return true
			}

			bc.OtherHeadNodesMutex.Lock()
			delete(bc.OtherHeadNodes, block0.PrevHash)
			bc.OtherHeadNodes[newBCN.Hash] = newBCN
			bc.OtherHeadNodesMutex.Unlock()
			bc.Miner.DebugPrint(fmt.Sprintf("-----block added to forked tree \n"))
			return false

		}
		bc.OtherHeadNodesMutex.Lock()
		delete(bc.OtherHeadNodes, block0.PrevHash)
		bc.OtherHeadNodes[newBCN.Hash] = newBCN
		bc.OtherHeadNodesMutex.Unlock()
		bc.Miner.DebugPrint(fmt.Sprintf("-----new fork created \n"))
		return false

	}

	bc.Head = newBCN
	bc.Miner.DebugPrint(fmt.Sprintf("-----added to main chain \n"))
	return true

}

// AddData joins the incoming data to the actual structures
func (bc *BlockChain) AddData(data []byte) {
	var Hash [32]byte
	temp := sha256.Sum256(data[:])
	copy(Hash[0:32], temp[0:32])

	bc.DanglingDataMutex.RLock()
	_, ok := bc.DanglingData[Hash]
	bc.DanglingDataMutex.RUnlock()
	if ok {
		bc.Miner.DebugPrint("data already known, but still dangling")
		return
	}

	bc.AllNodesMapMutex.Lock()
	val, ok := bc.AllNodesMap[Hash]
	bc.AllNodesMapMutex.Lock()

	if !ok {
		bc.DanglingDataMutex.Lock()
		bc.DanglingData[Hash] = data
		bc.DanglingDataMutex.Unlock()
		bc.Miner.DebugPrint("Added data as dangling")
	}

	tb := DeserializeTransactionBlock(data[:], bc)
	val.HasData = true
	val.DataPointer = tb

	bc.Miner.DebugPrint("added data to bcn, invoking the building of utxo")
}

// BlockChainGenesis creates first miner, mines first block and initiates the blockchain
func BlockChainGenesis(difficulty uint32, broadcaster *Broadcaster) *Miner {

	bl := new(BlockChainNode)
	bl.PrevBlockChainNode = bl
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
	bc.DanglingData = make(map[[32]byte][]byte)

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

//DeSerializeBlockChain get blockchain from byte array
func DeSerializeBlockChain(bc [][BlockSize]byte) *BlockChain {
	blockChain := new(BlockChain)
	blockChain.AllNodesMap = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanBlockChains = make([]*BlockChain, 0)
	blockChain.DanglingData = make(map[[32]byte][]byte)
	blockChain.IsOrphan = false

	bcn := new(BlockChainNode)
	bcn.Block = GenesisBlock
	bcn.Hash = GenesisBlock.Hash()
	bcn.PrevBlockChainNode = bcn

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.AllNodesMap[bcn.Hash] = bcn

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
	blockChain.OrphanBlockChains = nil //should never be called
	blockChain.IsOrphan = true
	blockChain.DanglingData = make(map[[32]byte][]byte)

	bcn := new(BlockChainNode)
	bcn.Block = bl
	bcn.Hash = bl.Hash()
	bcn.PrevBlockChainNode = nil

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.AllNodesMap[bcn.Hash] = bcn

	blockChain.Miner = bc.Miner

	return blockChain
}

// VerifyAndBuildDown request data if necesarry, builds utxo manager from bottum up from last know good state, redirects the upwards links an stores savepoints
func (bc *BlockChain) VerifyAndBuildDown(bcn *BlockChainNode) {

	if bc.IsOrphan {
		bc.Miner.DebugPrint("orphans cannot build utxo, returning ")
		return
	}

	if bcn.UTxOManagerIsUpToDate {
		bc.Miner.DebugPrint("txo manager already up to date")
		return
	}

	bcn.PrevBlockChainNode.NextBlockChainNode = bcn // to restore ability to crawl back up

	if !bcn.HasData {
		go bc.Miner.Request(RequestType["DataFromHash"], bc.Miner.Wallet.PublicKey, bcn.Hash[:])
		bc.Miner.DebugPrint("Requesting transactiondata")
		if !bcn.PrevBlockChainNode.UTxOManagerIsUpToDate {
			bc.VerifyAndBuildDown(bcn.PrevBlockChainNode)
		}
		// Else: can only wait for data
	} else {
		if bcn.PrevBlockChainNode.UTxOManagerIsUpToDate {

			goodTransactions := bcn.PrevBlockChainNode.UTxOManagerPointer.VerifyTransactionBlockRefs(bcn.DataPointer)
			if !goodTransactions {
				bcn.Badblock = true
				bc.Miner.DebugPrint("found bad block, transactions not matchin")
			}
			goodSignatures := bcn.DataPointer.VerifyInputSignatures()
			if !goodSignatures {
				bcn.Badblock = true
				bc.Miner.DebugPrint("False signatures, not continuing")
			}
			// good block, continuing
			keepCopy := bcn.Block.BlockCount%5 == 0
			bc.Miner.DebugPrint(fmt.Sprintf("Updating utxomanager, Keepcopy = %t", keepCopy))
			succes := bcn.PrevBlockChainNode.UTxOManagerPointer.UpdateWithNextBlockChainNode(bcn, bcn.Block.BlockCount%5 == 0)
			if !succes {
				log.Fatal("updating utxoManager Failed, should not happen!")
				return
			}
			bc.Miner.DebugPrint(fmt.Sprintf("Utxomanager is now at %d", bcn.Block.BlockCount))

		} else {
			bc.VerifyAndBuildDown(bcn.PrevBlockChainNode)
		}
	}

}
