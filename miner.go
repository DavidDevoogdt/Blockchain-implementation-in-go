package davidcoin

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Everyone is shorthand for all subscribed receivers
var Everyone [KeySize]byte

// Miner main type
type Miner struct {
	Name       string
	Wallet     *Wallet
	FileName   string
	BlockChain *BlockChain

	NetworkChannel chan []byte
	Broadcaster    *Broadcaster
	interrupt      chan bool

	Debug    bool
	isMining bool

	ReceivedData        map[[32]byte]bool
	ReceivedDataMutex   sync.Mutex
	ReceiveChannels     map[[32]byte]chan bool
	ReceiveChannelMutex sync.Mutex
}

// CreateMiner initiates miner with given broadcaster and blockchain
func CreateMiner(name string, Broadcaster *Broadcaster, blockChain [][BlockSize]byte) *Miner {

	m := new(Miner)
	m.Name = name
	m.Wallet = InitializeWallet()
	m.BlockChain = DeSerializeBlockChain(blockChain)
	m.Broadcaster = Broadcaster
	m.NetworkChannel = make(chan []byte)
	m.ReceiveChannels = make(map[[32]byte]chan bool)
	m.Broadcaster.append(m.NetworkChannel, m.Wallet.PublicKey)
	m.BlockChain.Miner = m
	m.interrupt = make(chan bool)
	m.Debug = false
	m.ReceivedData = make(map[[32]byte]bool)

	go m.ReceiveNetwork()

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
	m.NetworkChannel = make(chan []byte)
	m.ReceiveChannels = make(map[[32]byte]chan bool)
	m.Broadcaster.append(m.NetworkChannel, m.Wallet.PublicKey)
	m.interrupt = make(chan bool)
	m.Debug = false
	m.ReceivedData = make(map[[32]byte]bool)

	go m.ReceiveNetwork()
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

// RequestBlockFromHash makes request for block
func (m *Miner) RequestBlockFromHash(hash [32]byte) {
	m.Request(RequestType["BlockHeaderFromHash"], m.Wallet.PublicKey, hash[:])
}

// MineBlock mines block and add own credentials
func (m *Miner) MineBlock(data string) *Block {
	m.isMining = true
	//fmt.Printf("%s started mining %s\n", m.Name, data)
	prevBlock := m.BlockChain.Head.Block

	newBlock := new(Block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = rand.Uint32()
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
			//fmt.Printf("%s mined block: \n", m.kName)
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

//############################################

// Send broadcast block relevant addresses. Before transmitting the actual data, the receiver is requested to confirm whether he wants the data
func (m *Miner) Send(sendType uint8, data []byte, sender [KeySize]byte, receiver [KeySize]byte) {
	b := m.Broadcaster

	ss := new(SendStruct)
	ss.SendType = sendType
	ss.length = uint64(len(data))
	ss.data = data
	a := ss.SerializeSendStruct()

	rc, ok := b.Lookup[receiver]
	sd, _ := b.Lookup[sender]

	h := ss.ConfirmationHash()

	confirmSend := func(c chan []byte) {
		ll := make(chan bool)
		cs := CreateConfirmationStruct(h, sender)

		m.ReceiveChannelMutex.Lock()
		m.ReceiveChannels[cs.response] = ll
		m.ReceiveChannelMutex.Unlock()

		c <- SerializeNetworkMessage(NetworkTypes["Confirmation"], cs.SerializeConfirmationStruct())
		timer1 := time.NewTimer(2 * time.Second)
		select {
		case <-ll:
			c <- SerializeNetworkMessage(NetworkTypes["Send"], a)
		case <-timer1.C:
			//fmt.Printf("Rejected")
		}

		m.ReceiveChannelMutex.Lock()
		delete(m.ReceiveChannels, cs.response)
		m.ReceiveChannelMutex.Unlock()

		close(ll)
	}

	if !ok { // receiver unknown or everyone
		for _, c := range b.NetworkChannels {
			if c != sd {
				go confirmSend(c)
				//c <- SerializeNetworkMessage(NetworkTypes["Send"], a)
			}
		}
		return
	}
	go confirmSend(rc)
	//rc <- SerializeNetworkMessage(NetworkTypes["Send"], a)
}

// Request broadcast a request to everyone
func (m *Miner) Request(RequestType uint8, Requester [KeySize]byte, data []byte) {
	b := m.Broadcaster

	rq := new(RequestStruct)
	rq.RequestType = RequestType
	copy(rq.Requester[0:KeySize], Requester[0:KeySize])
	rq.length = uint64(len(data))
	rq.data = make([]byte, rq.length)
	copy(rq.data[0:rq.length], data[0:rq.length])

	a := rq.SerializeRequestStruct()

	//requester := b.Lookup[Requester]
	for _, c := range b.NetworkChannels {
		c <- SerializeNetworkMessage(NetworkTypes["Request"], a)
	}

}

//############################################

// ReceiveNetwork decodes all the incoming network traffic. The request are transferred to the coresponding decoders
func (m *Miner) ReceiveNetwork() {
	for {
		networkType, msg := DeserializeNetworkMessage(<-m.NetworkChannel)
		switch networkType {
		case NetworkTypes["Send"]:
			go m.ReceiveSend(msg)
		case NetworkTypes["Request"]:
			go m.ReceiveRequest(msg)
		case NetworkTypes["Confirmation"]:
			cs := DeserializeConfirmationStruct(msg)

			m.ReceivedDataMutex.Lock()
			_, ok := m.ReceivedData[cs.hash]
			m.ReceivedDataMutex.Unlock()
			if !ok {
				val, ok2 := m.Broadcaster.Lookup[cs.sender]
				if ok2 {
					val <- SerializeNetworkMessage(NetworkTypes["ConfirmationAccept"], cs.response[:])
				}
			}
		case NetworkTypes["ConfirmationAccept"]:
			var resp [32]byte
			copy(resp[0:32], msg[0:32])
			m.ReceiveChannelMutex.Lock()
			c, ok := m.ReceiveChannels[resp]
			m.ReceiveChannelMutex.Unlock()
			if ok {
				c <- true
			}

		}

	}
}

// ReceiveSend decodes the sent data and updates internal structure with the new dat
func (m *Miner) ReceiveSend(msg []byte) {

	ss := DeserializeSendStruct(msg)

	dataHash := sha256.Sum256(ss.data[:])

	m.ReceivedDataMutex.Lock()
	m.ReceivedData[dataHash] = true
	m.ReceivedDataMutex.Unlock()

	switch ss.SendType {
	case SendType["BlockHeader"]:
		var a [BlockSize]byte
		copy(a[0:BlockSize], ss.data[0:BlockSize])
		bl := DeserializeBlock(a)

		if m.BlockChain.addBlockChainNode(bl) {
			if m.isMining {
				m.interrupt <- true
			}
		}
	case SendType["BlockData"]:
		tbn := DeserializeTransactionBlockNode(ss.data[:])

		bcn := m.BlockChain.GetBlockChainNodeAtHash(tbn.Parent)

		if bcn != nil {
			if tbn.Hash == bcn.Block.Data {
				bcn.DataPointer = tbn.TransactionBlock
			} else {
				fmt.Printf("Received data of block with wrong Hash, not added")
			}

		}
	case SendType["HeaderAndData"]:
		var a [BlockSize]byte
		copy(a[0:BlockSize], ss.data[0:BlockSize])
		bl := DeserializeBlock(a)

		if m.BlockChain.addBlockChainNode(bl) {
			if m.isMining {
				m.interrupt <- true
			}
		}

		tbn := DeserializeTransactionBlockNode(ss.data[BlockSize:])
		bcn := m.BlockChain.GetBlockChainNodeAtHash(tbn.Parent)

		if bcn != nil {
			if tbn.Hash == bcn.Block.Data {
				bcn.DataPointer = tbn.TransactionBlock
			} else {
				fmt.Printf("Received data of block with wrong Hash, not added")
			}
		}
	case SendType["TransactionBlock"]:
		//tx := DeserializeTransactionBlock(ss.data)
		fmt.Printf("todo: implement this")

	}

}

// ReceiveRequest decodes the request and tries to service the request if possible
func (m *Miner) ReceiveRequest(msg []byte) {
	rq := DeserializeRequestStruct(msg)
	switch rq.RequestType {
	case RequestType["BlockHeaderFromHash"]:
		var data [32]byte
		copy(data[0:32], rq.data[0:32])
		bl := m.BlockChain.GetBlockChainNodeAtHash(data)
		//fmt.Printf("Got request from %x for block with hash %x\n", rq.Requester, data[:])

		if bl != nil {
			blData := bl.Block.SerializeBlock()
			go m.Send(SendType["BlockHeader"], blData[:], m.Wallet.PublicKey, rq.Requester)
		}
	case SendType["DataFromHash"]:
		var hash [32]byte
		copy(hash[0:32], rq.data[0:32])
		bl := m.BlockChain.GetBlockChainNodeAtHash(hash)
		if bl != nil {
			if bl.DataPointer != nil {
				blData := bl.DataPointer.SerializeTransactionBlock()
				go m.Send(SendType["BlockData"], blData[:], m.Wallet.PublicKey, rq.Requester)
			}
		}
	case SendType["Blockchain"]:
		fmt.Printf("---------------------Blockchain request not implemented!")
	}

}

// BroadcastBlock shorthand to broadcast block
func (m *Miner) BroadcastBlock(blc *Block) {
	data := blc.SerializeBlock()
	m.Send(SendType["BlockHeader"], data[:], m.Wallet.PublicKey, Everyone)
}
