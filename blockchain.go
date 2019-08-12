package main

import (
	"fmt"
)

// BlockChain is keeps track of known heads of tree
type BlockChain struct {
	Head                               *BlockChainNode
	allBlocksChainNodes                map[[32]byte]*BlockChainNode
	allOrphanBlocksChainNodes          map[[32]byte]*BlockChainNode
	OtherHeadBlocksChainNodes          map[[32]byte]*BlockChainNode
	OrphanedHeadBlocksChainNodes       map[[32]byte]*BlockChainNode
	OrphanedPrevRootBlocksChainNodes   map[[32]byte]*BlockChainNode
	OrphanedRootToHeadBlocksChainNodes map[[32]byte]*BlockChainNode
	OrphanedHeadToRootBlocksChainNodes map[[32]byte]*BlockChainNode
	Miner                              *Miner
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	PrevBlockChainNode *BlockChainNode
	Block              *Block
	Hash               [32]byte
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
	fmt.Printf("requested %d, got %d", height, current.Block.BlockCount)

	return current.Block
}

// GetBlockAtHash fetches block
func (bc *BlockChain) GetBlockAtHash(hash [32]byte) *Block {
	return bc.allBlocksChainNodes[hash].Block
}

func (bc *BlockChain) addBlockChainNode(block0 *Block) bool {
	k := bc.Miner
	var name string
	if k != nil {
		name = fmt.Sprintf("%s", bc.Miner.Name)
	} else {
		name = fmt.Sprintf("unknown")
	}

	if !block0.Verify() {
		fmt.Printf("-%s----Block not ok, not added\n", name)
		return false
	}

	_, nok := bc.allBlocksChainNodes[block0.Hash()]
	if nok {
		fmt.Printf("%s-----already in chain\n", name)
		return false
	}

	//Block.Print()
	//fmt.Printf("being added to blockchain \n")

	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()

	_Hash := newBCN.Hash
	_PrevHash := block0.PrevHash
	bc.allBlocksChainNodes[_Hash] = newBCN

	prevBlock, ok := bc.allBlocksChainNodes[_PrevHash]

	if !ok {

		fmt.Printf("%s####################prev Block not in history, requesting %.10x\n", name, _PrevHash)
		bc.OrphanedPrevRootBlocksChainNodes[_PrevHash] = newBCN
		_, ok = bc.OrphanedPrevRootBlocksChainNodes[_Hash]

		if ok {
			delete(bc.OrphanedPrevRootBlocksChainNodes, _Hash)
			fmt.Printf("%s##########orphaned chain grew one down\n", name)
			prevHead := bc.OrphanedRootToHeadBlocksChainNodes[_Hash]

			bc.OrphanedRootToHeadBlocksChainNodes[_PrevHash] = prevHead
			bc.OrphanedHeadToRootBlocksChainNodes[prevHead.Hash] = newBCN
			delete(bc.OrphanedRootToHeadBlocksChainNodes, _Hash)

		} else {
			bc.OrphanedHeadBlocksChainNodes[_Hash] = newBCN
			bc.OrphanedRootToHeadBlocksChainNodes[_PrevHash] = newBCN
			bc.OrphanedHeadToRootBlocksChainNodes[_Hash] = newBCN

			fmt.Printf("%s##########new orphaned block head \n", name)
		}

		go bc.Miner.HashRequestBlock(_PrevHash)
		return false

	}

	newBCN.PrevBlockChainNode = prevBlock

	if prevBlock.Hash != bc.Head.Hash {

		_, ok = bc.OrphanedPrevRootBlocksChainNodes[_Hash]

		if ok { // join orphaned tree to main tree

			_, ok1 := bc.OrphanedHeadBlocksChainNodes[_PrevHash]

			if ok1 { // attaching to other orphan
				delete(bc.OrphanedHeadBlocksChainNodes, _PrevHash)
				fmt.Printf("-##########----concat 2 orphaned chains \n")
				return false
			}

			// attaching to regular tree

			topBlock := bc.OrphanedPrevRootBlocksChainNodes[_Hash]
			delete(bc.OrphanedPrevRootBlocksChainNodes, _Hash)

			_, ok := bc.OtherHeadBlocksChainNodes[_PrevHash]

			if ok {
				delete(bc.OtherHeadBlocksChainNodes, _PrevHash)
			}

			if bc.Head.Block.BlockCount < topBlock.Block.BlockCount { // new best
				bc.OtherHeadBlocksChainNodes[bc.Head.Hash] = bc.Head
				bc.Head = newBCN
				fmt.Printf("##########-----orphaned block becomes main chain \n")
				return true
			}

			bc.OtherHeadBlocksChainNodes[topBlock.Hash] = topBlock
			fmt.Printf("##########-----joined orphaned branch to tree \n")
			return false

		}

		_, ok2 := bc.OrphanedHeadBlocksChainNodes[_PrevHash]
		if ok2 { //add to orphaned head
			root := bc.OrphanedHeadToRootBlocksChainNodes[_PrevHash]
			delete(bc.OrphanedHeadToRootBlocksChainNodes, _PrevHash)
			bc.OrphanedRootToHeadBlocksChainNodes[root.Hash] = newBCN
			bc.OrphanedHeadToRootBlocksChainNodes[_Hash] = root
			fmt.Printf("######### build block on orphaned brach\n")
			return false
		}

		_, ok = bc.OtherHeadBlocksChainNodes[_PrevHash]

		if ok { ////non primary chain got longer, check whether new longest
			if bc.Head.Block.BlockCount < block0.BlockCount { /////got new best
				bc.OtherHeadBlocksChainNodes[bc.Head.Hash] = bc.Head
				delete(bc.OtherHeadBlocksChainNodes, block0.PrevHash)
				bc.Head = newBCN
				fmt.Printf("-----block added as new head \n")
				return true
			}

			delete(bc.OtherHeadBlocksChainNodes, block0.PrevHash)
			bc.OtherHeadBlocksChainNodes[newBCN.Hash] = newBCN
			fmt.Printf("-----block added to forked tree \n")
			return false

		}
		delete(bc.OtherHeadBlocksChainNodes, block0.PrevHash)
		bc.OtherHeadBlocksChainNodes[newBCN.Hash] = newBCN
		fmt.Printf("-----new fork created \n")
		return false

	}

	_, ok3 := bc.OrphanedPrevRootBlocksChainNodes[_Hash]
	if ok3 { //orphaned chain  joins main chain
		top := bc.OrphanedRootToHeadBlocksChainNodes[_Hash]
		delete(bc.OrphanedRootToHeadBlocksChainNodes, _Hash)
		delete(bc.OrphanedHeadToRootBlocksChainNodes, top.Hash)
		bc.Head = top
		fmt.Printf("####### orphaned chain joins main \n")
		return true
	}

	bc.Head = newBCN
	fmt.Printf("-----added to main chain \n")
	return true

}

// BlockChainGenesis creates first miner, mines first block and initiates the blockchain
func BlockChainGenesis(difficulty uint32, broadcaster *Broadcaster) *Miner {

	bl := new(BlockChainNode)
	bl.PrevBlockChainNode = bl
	bc := new(BlockChain)
	bc.allBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.OtherHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.OrphanedRootToHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.OrphanedHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.OrphanedHeadToRootBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.OrphanedPrevRootBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	bc.allOrphanBlocksChainNodes = make(map[[32]byte]*BlockChainNode)

	genesisMiner := CreateGenesisMiner("genesis", broadcaster, bc)
	bc.Miner = genesisMiner
	gen := generateGenesis(difficulty, genesisMiner)
	bl.Block = gen
	bl.Hash = bl.Block.Hash()
	bc.Head = bl

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

	BlockChainNode := bc.Head
	BlockChainNode.Block.Print()
	for BlockChainNode.Block.BlockCount != 0 {
		BlockChainNode = BlockChainNode.PrevBlockChainNode
		BlockChainNode.Block.Print()

	}
}

// PrintHash print
func (bc *BlockChain) PrintHash() {

	BlockChainNode := bc.Head
	fmt.Printf("%d:%x\n", BlockChainNode.Block.BlockCount, BlockChainNode.Block.Hash())
	for BlockChainNode.Block.BlockCount != 0 {
		BlockChainNode = BlockChainNode.PrevBlockChainNode
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
	blockChain.allBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanedPrevRootBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanedHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanedRootToHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanedHeadToRootBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.allOrphanBlocksChainNodes = make(map[[32]byte]*BlockChainNode)

	bcn := new(BlockChainNode)
	bcn.Block = GenesisBlock
	bcn.Hash = GenesisBlock.Hash()
	bcn.PrevBlockChainNode = bcn

	blockChain.Head = bcn
	blockChain.allBlocksChainNodes[bcn.Hash] = bcn

	//first block

	if len(bc) >= 1 {
		for _, bl := range bc[1:] {
			blockChain.addBlockChainNode(deSerialize(bl))
		}
	}

	return blockChain
}
