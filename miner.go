package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

//import "crypto/rsa"

// Miner main type
type Miner struct {
	Name       string
	Wallet     [8]byte
	FileName   string
	BlockChain *BlockChain
	PublicKey  []byte
	PrivateKey []byte

	ReceiveChannel     chan [BlockSize]byte
	IntRequestChannel  chan IntRequest
	HashRequestChannel chan HashRequest
	Broadcaster        *Broadcaster
	interrupt          chan bool
}

// CreateMiner initates miner with given broadcaster and blockchain
func CreateMiner(name string, Broadcaster *Broadcaster, blockChain [][BlockSize]byte) *Miner {

	m := new(Miner)
	m.Name = name
	foo := make([]byte, 8)
	rand.Read(foo)
	copy(m.Wallet[:], foo[0:7])
	priv, pub := GenerateKeyPair(16)

	m.PrivateKey = PrivateKeyToBytes(priv)
	m.PublicKey = PublicKeyToBytes(pub)
	m.BlockChain = DeSerializeBlockChain(blockChain)
	m.Broadcaster = Broadcaster

	m.ReceiveChannel = make(chan [BlockSize]byte)
	m.IntRequestChannel = make(chan IntRequest)
	m.HashRequestChannel = make(chan HashRequest)
	m.Broadcaster.append(m.ReceiveChannel, m.IntRequestChannel, m.HashRequestChannel)

	m.BlockChain.Miner = m
	m.interrupt = make(chan bool)

	go m.ReceiveBlocks()

	return m
}

/*
func MinerFromScratch() *BlockChain {
	blockChain := new(BlockChain)
	blockChain.allBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OtherHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanedHeadBlocksChainNodes = make(map[[32]byte]*BlockChainNode)
	blockChain.OrphanedRootBlocksChainNodes = make(map[[32]byte]*BlockChainNode)




	return m

}
*/

// CreateGenesisMiner used for first miners
func CreateGenesisMiner(name string, Broadcaster *Broadcaster, blockChain *BlockChain) *Miner {

	m := new(Miner)
	m.Name = name
	foo := make([]byte, 8)
	rand.Read(foo)
	copy(m.Wallet[:], foo[0:7])
	priv, pub := GenerateKeyPair(16)

	m.PrivateKey = PrivateKeyToBytes(priv)
	m.PublicKey = PublicKeyToBytes(pub)
	m.BlockChain = blockChain
	m.Broadcaster = Broadcaster

	m.ReceiveChannel = make(chan [BlockSize]byte)
	m.IntRequestChannel = make(chan IntRequest)
	m.HashRequestChannel = make(chan HashRequest)
	m.Broadcaster.append(m.ReceiveChannel, m.IntRequestChannel, m.HashRequestChannel)
	m.interrupt = make(chan bool)
	go m.ReceiveBlocks()

	return m
}

// ReceiveBlocks should be called once to receive blocks from broadcasters
func (m *Miner) ReceiveBlocks() {
	for {
		a := <-m.ReceiveChannel
		bl := deSerialize(a)
		fmt.Printf("miner %s reciever request to add %.10x\n", m.Name, a)

		m.interrupt <- true

		m.BlockChain.addBlockChainNode(bl)
	}
}
func (m *Miner) intRequests() {
	for {
		a := <-m.IntRequestChannel
		fmt.Printf("%s received request for block %d\n", m.Name, a.Height)
	}
}

func (m *Miner) hashRequests() {
	for {
		a := <-m.HashRequestChannel
		fmt.Printf("%s received request for block %x\n", m.Name, a.Hash)
	}
}

// BroadcastBlock let miner broadcast block to pool
func (m *Miner) BroadcastBlock(block0 *Block) {
	m.Broadcaster.SendBlock(block0.serialize(), m.ReceiveChannel)
}

// IntRequestBlock let miner request block with specific number
func (m *Miner) IntRequestBlock(number uint32) {
	m.Broadcaster.IntRequestBlock(NewIntRequest(number, m.ReceiveChannel))
}

// MineBlock mines block and add own credentials
func (m *Miner) MineBlock(data string) *Block {

	//fmt.Printf("%s started mining %s\n", m.Name, data)
	prevBlock := m.BlockChain.Head.Block

	newBlock := new(Block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = 0
	newBlock.Data = sha256.Sum256([]byte(data))
	newBlock.Difficulty = prevBlock.Difficulty

	copy(newBlock.Owner[:], m.Wallet[:])

	for {
		select {
		case <-m.interrupt:
			//fmt.Printf("interupted %t")
			return nil
		default:
			if compHash(newBlock.Difficulty, newBlock.Hash()) {
				return newBlock
			}
			newBlock.Nonce++
		}
	}
}

// MineContiniously does what it says
func (m *Miner) MineContiniously() {
	for {
		blc := m.MineBlock(fmt.Sprintf("dit is blok nr %d", m.BlockChain.Head.Block.BlockCount))
		print(blc)
		if blc == nil {
			fmt.Printf("%s was not fast enough \n", m.Name)
		} else {
			fmt.Printf("%s mined block: \n", m.Name)
			m.BroadcastBlock(blc)
			m.BlockChain.addBlockChainNode(blc)
		}
	}
}

// Print print
func (m *Miner) Print() {
	fmt.Printf("\n-----------------------------\n")
	fmt.Printf("name miner: %s\n", m.Name)
	m.BlockChain.Print()
	fmt.Printf("\n-----------------------------\n")
}

// PrintHash print
func (m *Miner) PrintHash() {
	fmt.Printf("\n-----------------------------\n")
	fmt.Printf("name miner: %s\n", m.Name)
	m.BlockChain.PrintHash()
	fmt.Printf("\n-----------------------------\n")
}
