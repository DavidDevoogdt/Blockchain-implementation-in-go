package main

import (
	"fmt"
)

// BlockChain is keeps track of known heads of tree
type BlockChain struct {
	Head                      *BlockChainNode
	allBlocksChainNodes       map[[32]byte]*BlockChainNode
	OtherHeadBlocksChainNodes map[[32]byte]*BlockChainNode
	Miner                     *Miner
	OrphanBlockChains         []*BlockChain
	IsOrphan                  bool
	Root                      *BlockChainNode
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	PrevBlockChainNode *BlockChainNode
	Block              *Block
	Hash               [32]byte
}

// HasBlock determines whether block is in chain
func (bc *BlockChain) HasBlock(hash [32]byte) bool {
	_, ok := bc.allBlocksChainNodes[hash]
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
	return bc.allBlocksChainNodes[hash].Block
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

	_, nok := bc.allBlocksChainNodes[block0.Hash()]
	if nok {
		bc.Miner.DebugPrint(fmt.Sprintf("%s-----already in chain\n", name))
		return false
	}

	_Hash := block0.Hash()
	_PrevHash := block0.PrevHash

	prevBlock, ok := bc.allBlocksChainNodes[_PrevHash]

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
				obc.Root = newBCN
				obc.allBlocksChainNodes[_Hash] = newBCN

				go bc.Miner.HashRequestBlock(block0.PrevHash)
				bc.Miner.DebugPrint(fmt.Sprintf("%s-----setting new root for orphaned chain \n", name))
				return false
			}
		}

		bc.Miner.DebugPrint(fmt.Sprintf("%s-----prev Block not in history, requesting %.10x\n", name, _PrevHash))
		bc.Miner.DebugPrint(fmt.Sprintf("%s-----creating new orphaned chain \n", name))
		bc.OrphanBlockChains = append(bc.OrphanBlockChains, bc.InitializeOrphanBlockChain(block0))
		go bc.Miner.HashRequestBlock(_PrevHash)
		return false

	}
	// prevhash in this chain, create new blockNode
	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()
	bc.allBlocksChainNodes[_Hash] = newBCN

	newBCN.PrevBlockChainNode = prevBlock

	// check whether hash is contained as prehash in orphaned chains
	for i, obc := range bc.OrphanBlockChains {
		if _Hash == obc.Root.Block.PrevHash {
			bc.Miner.DebugPrint(fmt.Sprintf("%s-----joining orphaned chain to main chain!\n", name))
			obc.Root.PrevBlockChainNode = newBCN

			// delete prev node from head if that were the case
			delete(bc.OtherHeadBlocksChainNodes, _PrevHash)
			//merge hashmaps
			for k, v := range obc.OtherHeadBlocksChainNodes {
				bc.OtherHeadBlocksChainNodes[k] = v
			}
			for k, v := range obc.allBlocksChainNodes {
				bc.allBlocksChainNodes[k] = v
			}

			// put orphaned blockchain to last element and remove it
			l := len(bc.OrphanBlockChains)
			bc.OrphanBlockChains[i] = bc.OrphanBlockChains[l-1]
			bc.OrphanBlockChains = bc.OrphanBlockChains[:l-1]

			mainHead := bc.Head
			orphanHead := obc.Head
			if orphanHead.Block.BlockCount > bc.Head.Block.BlockCount {

				bc.OtherHeadBlocksChainNodes[mainHead.Hash] = mainHead
				bc.Head = orphanHead
				bc.Miner.DebugPrint(fmt.Sprintf("%s-----orphaned chain is now main chain\n", name))
				return true
			}

			bc.OtherHeadBlocksChainNodes[orphanHead.Hash] = orphanHead
			return false

		}
	}

	//nothing special, just main chain
	if prevBlock.Hash != bc.Head.Hash {
		_, ok = bc.OtherHeadBlocksChainNodes[_PrevHash]

		if ok { ////non primary chain got longer, check whether new longest
			if bc.Head.Block.BlockCount < block0.BlockCount { /////got new best
				bc.OtherHeadBlocksChainNodes[bc.Head.Hash] = bc.Head
				delete(bc.OtherHeadBlocksChainNodes, block0.PrevHash)
				bc.Head = newBCN
				bc.Miner.DebugPrint(fmt.Sprintf("-----block added as new head \n"))
				return true
			}

			delete(bc.OtherHeadBlocksChainNodes, block0.PrevHash)
			bc.OtherHeadBlocksChainNodes[newBCN.Hash] = newBCN
			bc.Miner.DebugPrint(fmt.Sprintf("-----block added to forked tree \n"))
			return false

		}
		delete(bc.OtherHeadBlocksChainNodes, block0.PrevHash)
		bc.OtherHeadBlocksChainNodes[newBCN.Hash] = newBCN
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
	bc.allBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.OtherHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
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
	bc.allBlocksChainNodes[bl.Hash] = bl

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
	bc.print("")
}

func (bc *BlockChain) print(prefix string) {

	BlockChainNode := bc.Head
	BlockChainNode.Block.print(prefix)
	for BlockChainNode.Block.BlockCount != 0 {
		BlockChainNode = BlockChainNode.PrevBlockChainNode
		BlockChainNode.Block.print(prefix)
	}

	for _, obc := range bc.OrphanBlockChains {
		obc.print(prefix + "---")
	}

}

// PrintHash print
func (bc *BlockChain) PrintHash() {
	bc.printHash("")
}

func (bc *BlockChain) printHash(prefix string) {

	BlockChainNode := bc.Head
	fmt.Printf("%s%d:%x\n", prefix, BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())
	for BlockChainNode != bc.Root {
		BlockChainNode = BlockChainNode.PrevBlockChainNode
		fmt.Printf("%s%d:%x\n", prefix, BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())

	}

	for _, obc := range bc.OrphanBlockChains {
		obc.printHash(prefix + "---")
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
	blockChain.allBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanBlockChains = make([]*BlockChain, 0)
	blockChain.IsOrphan = false

	bcn := new(BlockChainNode)
	bcn.Block = GenesisBlock
	bcn.Hash = GenesisBlock.Hash()
	bcn.PrevBlockChainNode = bcn

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.allBlocksChainNodes[bcn.Hash] = bcn

	//first block

	for _, bl := range bc[:] {
		blockChain.addBlockChainNode(deSerialize(bl))
	}

	return blockChain
}

// InitializeOrphanBlockChain generates pointer to new bc
func (bc *BlockChain) InitializeOrphanBlockChain(bl *Block) *BlockChain {
	blockChain := new(BlockChain)
	blockChain.allBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanBlockChains = nil //should never be called
	blockChain.IsOrphan = true

	bcn := new(BlockChainNode)
	bcn.Block = bl
	bcn.Hash = bl.Hash()
	bcn.PrevBlockChainNode = nil

	blockChain.Head = bcn
	blockChain.Root = bcn
	blockChain.allBlocksChainNodes[bcn.Hash] = bcn

	blockChain.Miner = bc.Miner

	return blockChain
}
