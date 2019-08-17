package main

import (
	"crypto/sha256"
	"fmt"
)

// Everyone is shorthand for all subscribed receivers
var Everyone [KeySize]byte

// Miner main type
type Miner struct {
	Name       string
	Wallet     *Wallet
	FileName   string
	BlockChain *BlockChain

	ReceiveChannel chan []byte
	RequestChannel chan []byte

	Broadcaster *Broadcaster
	interrupt   chan bool

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

	m.ReceiveChannel = make(chan []byte)
	m.RequestChannel = make(chan []byte)
	m.Broadcaster.append(m.ReceiveChannel, m.RequestChannel, m.Wallet.PublicKey)

	m.BlockChain.Miner = m
	m.interrupt = make(chan bool)
	m.Debug = false

	go m.ReceiveSend()
	go m.ReceiveRequest()

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

	m.ReceiveChannel = make(chan []byte)
	m.RequestChannel = make(chan []byte)
	m.Broadcaster.append(m.ReceiveChannel, m.RequestChannel, m.Wallet.PublicKey)

	m.interrupt = make(chan bool)
	go m.ReceiveSend()
	go m.ReceiveRequest()

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

// ReceiveSend should be called once to receive blocks from broadcasters
func (m *Miner) ReceiveSend() {
	for {
		ss := DeserializeSendStruct(<-m.ReceiveChannel)

		switch ss.SendType {
		case SendType["BlockHeader"]:
			var a [BlockSize]byte
			//fmt.Printf("data: %x\n", ss.data[:])
			copy(a[0:BlockSize], ss.data[0:BlockSize])
			bl := deSerialize(a)
			//fmt.Printf("miner %s receiver request to add %.10x\n", m.Name, a)

			if m.BlockChain.addBlockChainNode(bl) {
				if m.isMining {
					m.interrupt <- true
				}

			}

		}

	}
}

// ReceiveRequest ...
func (m *Miner) ReceiveRequest() {
	for {
		a := <-m.RequestChannel
		//fmt.Printf("%x\n", a)
		rq := DeserializeRequestStruct(a)

		switch rq.RequestType {
		case SendType["BlockHeaderFromHash"]:
			var data [32]byte
			copy(data[0:32], rq.data[0:32])
			bl := m.BlockChain.GetBlockAtHash(data)
			//fmt.Printf("Got request from %x for block with hash %x\n", rq.Requester, data[:])

			if bl != nil {
				blData := bl.serialize()
				go m.Broadcaster.Send(SendType["BlockHeader"], blData[:], m.Wallet.PublicKey, rq.Requester)
			}
		}

	}
}

// BroadcastBlock shorthand to broadcast block
func (m *Miner) BroadcastBlock(blc *Block) {
	data := blc.serialize()
	m.Broadcaster.Send(SendType["BlockHeader"], data[:], m.Wallet.PublicKey, Everyone)
}

// RequestBlockFromHash makes request for block
func (m *Miner) RequestBlockFromHash(hash [32]byte) {
	m.Broadcaster.Request(RequestType["BlockHeaderFromHash"], m.Wallet.PublicKey, hash[:])
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
	k := m.Wallet.PublicKey
	fmt.Printf("name miner: %s (%x)\n", m.Name, k[len(k)-10:])
	m.BlockChain.PrintHash(n)
	fmt.Printf("\n-----------------------------\n")

}
