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
}

// BlockChainNode links block to previous parent
type BlockChainNode struct {
	PrevBlockChainNode *BlockChainNode
	Block              *Block
	Hash               [32]byte
}

func (bc *BlockChain) addBlockChainNode(block0 *Block) bool {
	k := bc.Miner
	if k != nil {
		fmt.Printf("%s", bc.Miner.Name)
	} else {
		fmt.Printf("unknown")
	}

	if !block0.Verify() {
		fmt.Printf("-----Block not ok, not added\n")
		return false
	}

	_, nok := bc.allBlocksChainNodes[block0.Hash()]
	if nok {
		fmt.Printf("-----already in chain\n")
		return false
	}

	//Block.Print()
	//fmt.Printf("being added to blockchain \n")

	newBCN := new(BlockChainNode)
	newBCN.Block = block0
	newBCN.Hash = block0.Hash()

	prevBlock, ok := bc.allBlocksChainNodes[block0.PrevHash]

	bc.allBlocksChainNodes[newBCN.Hash] = newBCN

	if !ok {
		//todo implement this
		fmt.Printf("-----Block not in history, implement request to sender for previous block\nhash: %x\n", block0.PrevHash)

	}
	newBCN.PrevBlockChainNode = prevBlock

	if prevBlock.Hash != bc.Head.Hash {
		_, ok = bc.OtherHeadBlocksChainNodes[block0.PrevHash]
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

	nBlock1 := deSerialize(bc[0])

	bcn := new(BlockChainNode)
	bcn.Block = nBlock1
	bcn.Hash = nBlock1.Hash()
	bcn.PrevBlockChainNode = bcn

	blockChain.Head = bcn
	blockChain.allBlocksChainNodes[bcn.Hash] = bcn

	//first block

	for _, bl := range bc[1:] {

		nBlock := deSerialize(bl)

		blockChain.addBlockChainNode(nBlock)

	}

	return blockChain
}
