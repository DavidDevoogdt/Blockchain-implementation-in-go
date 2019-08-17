package main

import (
	"fmt"
)

// BlockChain is keeps track of known heads of tree
type BlockChain struct {
	Head              *BlockChainNode
	Root              *BlockChainNode
	AllNodesMap       map[[32]byte]*BlockChainNode
	OtherHeadNodes    map[[32]byte]*BlockChainNode
	Miner             *Miner
	OrphanBlockChains []*BlockChain
	IsOrphan          bool
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	PrevBlockChainNode *BlockChainNode
	NextBlockChainNode *BlockChainNode
	Block              *Block
	Hash               [32]byte
}

// HasBlock determines whether block is in chain
func (bc *BlockChain) HasBlock(hash [32]byte) bool {
	_, ok := bc.AllNodesMap[hash]
	return ok
}

// GetBlockAtHeight returns block of main chain at specific height
func (bc *BlockChain) GetBlockAtHeight(height uint32) *Block {
	current := bc.Head
	max := current.Block.BlockCount
	if height > max {
		return nil
	}

	for i := uint32(0); i < max-height; i++ {
		current = current.PrevBlockChainNode
	}

	//fmt.Printf("requested %d, got %d", height, current.Block.BlockCount)

	return current.Block
}

// GetBlockAtHash fetches block
func (bc *BlockChain) GetBlockAtHash(hash [32]byte) *Block {
	val, ok := bc.AllNodesMap[hash]
	if ok {
		return val.Block
	}
	return nil
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

	_, nok := bc.AllNodesMap[block0.Hash()]
	if nok {
		bc.Miner.DebugPrint(fmt.Sprintf("%s-----already in chain\n", name))
		return false
	}

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash

	prevBlock, ok := bc.AllNodesMap[_PrevHash]

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
				obc.AllNodesMap[_Hash] = newBCN

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
	bc.AllNodesMap[_Hash] = newBCN

	newBCN.PrevBlockChainNode = prevBlock
	prevBlock.NextBlockChainNode = newBCN

	// check whether hash is contained as prehash in orphaned chains
	for i, obc := range bc.OrphanBlockChains {
		if _Hash == obc.Root.Block.PrevHash {
			bc.Miner.DebugPrint(fmt.Sprintf("%s-----joining orphaned chain to main chain!\n", name))
			obc.Root.PrevBlockChainNode = newBCN

			// delete prev node from head if that were the case
			delete(bc.OtherHeadNodes, _PrevHash)
			//merge hashmaps
			for k, v := range obc.OtherHeadNodes {
				bc.OtherHeadNodes[k] = v
			}
			for k, v := range obc.AllNodesMap {
				bc.AllNodesMap[k] = v
			}

			// put orphaned blockchain to last element and remove it
			l := len(bc.OrphanBlockChains)
			bc.OrphanBlockChains[i] = bc.OrphanBlockChains[l-1]
			bc.OrphanBlockChains = bc.OrphanBlockChains[:l-1]

			mainHead := bc.Head
			orphanHead := obc.Head
			if orphanHead.Block.BlockCount > bc.Head.Block.BlockCount {

				bc.OtherHeadNodes[mainHead.Hash] = mainHead
				bc.Head = orphanHead
				bc.Miner.DebugPrint(fmt.Sprintf("%s-----orphaned chain is now main chain\n", name))
				return true
			}

			bc.OtherHeadNodes[orphanHead.Hash] = orphanHead
			return false

		}
	}

	//nothing special, just main chain
	if prevBlock.Hash != bc.Head.Hash {
		_, ok = bc.OtherHeadNodes[_PrevHash]

		if ok { ////non primary chain got longer, check whether new longest
			if bc.Head.Block.BlockCount < block0.BlockCount { /////got new best
				bc.OtherHeadNodes[bc.Head.Hash] = bc.Head
				delete(bc.OtherHeadNodes, block0.PrevHash)
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

			delete(bc.OtherHeadNodes, block0.PrevHash)
			bc.OtherHeadNodes[newBCN.Hash] = newBCN
			bc.Miner.DebugPrint(fmt.Sprintf("-----block added to forked tree \n"))
			return false

		}
		delete(bc.OtherHeadNodes, block0.PrevHash)
		bc.OtherHeadNodes[newBCN.Hash] = newBCN
		bc.Miner.DebugPrint(fmt.Sprintf("-----new fork created \n"))
		return false

	}

	bc.Head = newBCN
	bc.Miner.DebugPrint(fmt.Sprintf("-----added to main chain \n"))
	return true

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

	genesisMiner := CreateGenesisMiner("genesis", broadcaster, bc)
	bc.Miner = genesisMiner
	gen := generateGenesis(difficulty, genesisMiner)
	bl.Block = gen
	bl.Hash = bl.Block.Hash()
	bc.Head = bl
	bc.Root = bl
	bc.Miner.Debug = false
	bc.AllNodesMap[bl.Hash] = bl

	return genesisMiner
}

// Verify checks to blockchain from head to root
func (bc *BlockChain) Verify() bool {
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
		ret[current.Block.BlockCount] = current.Block.serialize()
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
	blockChain.IsOrphan = false

	bcn := new(BlockChainNode)
	bcn.Block = GenesisBlock
	bcn.Hash = GenesisBlock.Hash()
	bcn.PrevBlockChainNode = bcn

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.AllNodesMap[bcn.Hash] = bcn

	//first block

	for _, bl := range bc[:] {
		blockChain.addBlockChainNode(deSerialize(bl))
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
