package main

import (
	"crypto/sha256"
	"fmt"
)

//import "crypto/rsa"

// Miner main type
type Miner struct {
	Name       string
	Wallet     *Wallet
	FileName   string
	BlockChain *BlockChain

	ReceiveChannel     chan [BlockSize]byte
	IntRequestChannel  chan IntRequest
	HashRequestChannel chan HashRequest
	Broadcaster        *Broadcaster
	interrupt          chan bool

	Debug    bool
	isMining bool
}

// CreateMiner initiates miner with given broadcaster and blockchain
func CreateMiner(name string, Broadcaster *Broadcaster, blockChain [][BlockSize]byte) *Miner {

	m := new(Miner)
	m.Name = name

	m.Wallet = InitializeWallet()

	m.BlockChain = DeSerializeBlockChain(blockChain)

	m.Broadcaster = Broadcaster

	m.ReceiveChannel = make(chan [BlockSize]byte)
	m.IntRequestChannel = make(chan IntRequest)
	m.HashRequestChannel = make(chan HashRequest)
	m.Broadcaster.append(m.ReceiveChannel, m.IntRequestChannel, m.HashRequestChannel)

	m.BlockChain.Miner = m
	m.interrupt = make(chan bool)
	m.Debug = false

	go m.ReceiveBlocks()

	return m
}

// MinerFromScratch ask for own resources for generation of blockchain
func MinerFromScratch(name string, broadcaster *Broadcaster) *Miner {
	m := CreateMiner(name, broadcaster, make([][BlockSize]byte, 0))
	//go m.IntRequestBlock(1)
	return m
}

// CreateGenesisMiner used for first miners
func CreateGenesisMiner(name string, Broadcaster *Broadcaster, blockChain *BlockChain) *Miner {

	m := new(Miner)
	m.Name = name
	m.Wallet = InitializeWallet()

	m.BlockChain = blockChain
	m.Broadcaster = Broadcaster

	m.ReceiveChannel = make(chan [BlockSize]byte)
	m.IntRequestChannel = make(chan IntRequest)
	m.HashRequestChannel = make(chan HashRequest)
	m.Broadcaster.append(m.ReceiveChannel, m.IntRequestChannel, m.HashRequestChannel)
	m.interrupt = make(chan bool)
	go m.ReceiveBlocks()
	go m.intRequests()
	go m.hashRequests()
	m.Debug = false

	return m
}

//DebugPrint print if debug is on
func (m *Miner) DebugPrint(msg string) {
	if m != nil {
		if m.Debug {
			fmt.Printf(msg)
		}
	}
}

// StartDebug starts debug logging
func (m *Miner) StartDebug() {
	m.Debug = true
}

// ReceiveBlocks should be called once to receive blocks from broadcasters
func (m *Miner) ReceiveBlocks() {
	for {
		a := <-m.ReceiveChannel
		bl := deSerialize(a)
		//fmt.Printf("miner %s receiver request to add %.10x\n", m.Name, a)

		if m.BlockChain.addBlockChainNode(bl) {
			if m.isMining {
				m.interrupt <- true
			}

		}

	}
}

func (m *Miner) intRequests() {
	for {
		a := <-m.IntRequestChannel
		bl := m.BlockChain.GetBlockAtHeight(a.Height)
		if bl != nil {
			a.requester <- bl.serialize()
		}
	}
}

func (m *Miner) hashRequests() {
	for {
		a := <-m.HashRequestChannel
		//fmt.Printf("Minerpool received hashRequest for block %x", a.Hash)
		b := m.BlockChain.GetBlockAtHash(a.Hash)
		if b != nil {
			a.requester <- b.serialize()
		}
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

// HashRequestBlock let miner request block with specific number
func (m *Miner) HashRequestBlock(hash [32]byte) {
	m.Broadcaster.HashRequestBlock(NewHashRequest(hash, m.ReceiveChannel))
}

// MineBlock mines block and add own credentials
func (m *Miner) MineBlock(data string) *Block {
	m.isMining = true
	//fmt.Printf("%s started mining %s\n", m.Name, data)
	prevBlock := m.BlockChain.Head.Block

	newBlock := new(Block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = 0
	newBlock.Data = sha256.Sum256([]byte(data))
	newBlock.Difficulty = prevBlock.Difficulty

	for {
		select {
		case <-m.interrupt:
			m.isMining = false
			return nil
		default:
			if compHash(newBlock.Difficulty, newBlock.Hash()) {

				m.isMining = false
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
		//print(blc)
		if blc == nil {
			//fmt.Printf("%s was not fast enough \n", m.Name)
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
func (m *Miner) PrintHash(n int) {
	fmt.Printf("\n-----------------------------\n")
	fmt.Printf("name miner: %s\n", m.Name)
	m.BlockChain.PrintHash(n)
	fmt.Printf("\n-----------------------------\n")
}
