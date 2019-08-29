package davidcoin

import (
	"crypto/sha256"
	"encoding/binary"
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

	GeneralMutex sync.Mutex

	ReceivedData      map[[32]byte]bool
	ReceivedDataMutex sync.Mutex

	ReceiveChannels     map[[32]byte]chan bool
	ReceiveChannelMutex sync.Mutex

	SynchronousReceiveChannels      map[[32]byte]chan []byte
	SynchronousReceiveChannelsMutex sync.Mutex
}

//###################one time init###################

// CreateMiner initiates miner with given broadcaster and blockchain
func CreateMiner(name string, Broadcaster *Broadcaster, blockChain [][BlockSize]byte) *Miner {

	m := new(Miner)
	m.Name = name
	m.Wallet = InitializeWallet()
	m.BlockChain = DeserializeBlockChain(blockChain)
	m.Broadcaster = Broadcaster
	m.NetworkChannel = make(chan []byte)
	m.ReceiveChannels = make(map[[32]byte]chan bool)
	m.Broadcaster.append(m.NetworkChannel, m.Wallet.PublicKey)
	m.BlockChain.Miner = m
	m.interrupt = make(chan bool)
	m.Debug = false
	m.ReceivedData = make(map[[32]byte]bool)
	m.BlockChain.Root.UTxOManagerPointer = InitializeUTxOMananger(m)
	m.BlockChain.Root.UTxOManagerIsUpToDate = true
	//InitializeUTxOMananger(m)

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

	//InitializeUTxOMananger(m)

	go m.ReceiveNetwork()
	return m
}

// StartDebug starts debug logging
func (m *Miner) StartDebug() {
	m.GeneralMutex.Lock()
	m.Debug = true
	m.GeneralMutex.Unlock()
}

//###############actual mining #######################""

// MineBlock mines block and add own credentials
func (m *Miner) MineBlock(newBlock *Block) *Block {
	m.GeneralMutex.Lock()
	m.isMining = true
	m.GeneralMutex.Unlock()

	defer func() {
		m.GeneralMutex.Lock()
		m.isMining = false
		m.GeneralMutex.Unlock()
	}()

	for {
		select {
		case <-m.interrupt:
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
	i := uint32(0)
	for {
		tb := InitializeTransactionBlock()
		copy(tb.PayerPublicKey[0:KeySize], m.Wallet.PublicKey[0:KeySize])

		to := &TransactionOutput{Amount: 1 * davidcoin}
		copy(to.ReceiverPublicKey[0:KeySize], m.Wallet.PublicKey[0:KeySize])
		binary.LittleEndian.PutUint32(to.text[0:4], i)
		to.Sign(m.Wallet.PrivateKey)

		tb.AddOutput(to)
		tb.SignBlock(m.Wallet.PrivateKey)

		tbg := InitializeTransactionBlockGroup()
		tbg.Add(tb)
		tbg.FinalizeTransactionBlockGroup()

		tbg.SerializeTransactionBlockGroup()

		prepBlock := m.PrepareBlockForMining(tbg)

		if prepBlock != nil {
			blc := m.MineBlock(prepBlock)
			//print(blc)
			if blc == nil {
				m.DebugPrint(fmt.Sprintf("%s was not fast enough \n", m.Name))
			} else {
				m.DebugPrint(fmt.Sprintf("%s mined block: \n", m.Name))
				m.BlockChain.AddInternal(blc, tbg)

				m.BroadcastBlock(blc)
			}

		} else {
			m.DebugPrint("Block nil, somthing went wrong with preparation of it\n")
		}
	}
}

// PrepareBlockForMining takes transactiondata and builds a block suitable for mining
func (m *Miner) PrepareBlockForMining(tbg *TransactionBlockGroup) *Block {

	m.DebugPrint(" started preparing tbg for mining\n")

	m.GeneralMutex.Lock()
	bc := m.BlockChain
	m.GeneralMutex.Unlock()

	bc.BlockChainMutex.Lock()
	head := bc.Head
	bc.BlockChainMutex.Unlock()

	head.generalMutex.RLock()
	headUTxOManagerIsUpToDate := head.UTxOManagerIsUpToDate
	headUTxOManagerPointer := head.UTxOManagerPointer
	prevBlock := head.Block
	head.generalMutex.RUnlock()

	if !headUTxOManagerIsUpToDate {
		m.DebugPrint(fmt.Sprintf("head is not up to date, invoking and returning\n"))
	}

	goodSign := make(chan bool)
	goodRef := make(chan bool)

	go func() {
		goodSign <- tbg.VerifyExceptUTXO()
	}()

	go func() {
		goodRef <- headUTxOManagerPointer.VerifyTransactionBlockRefs(tbg)
	}()

	//m.DebugPrint(fmt.Sprintf("newHash is %x", newHash))
	//m.DebugPrint(fmt.Sprintf("%s started mining %x\n", m.Name, newHash))

	newBlock := new(Block)
	newBlock.BlockCount = prevBlock.BlockCount + 1
	newBlock.PrevHash = prevBlock.Hash()
	newBlock.Nonce = rand.Uint32()
	newBlock.MerkleRoot = tbg.merkleTree.GetMerkleRoot()
	newBlock.Difficulty = prevBlock.Difficulty

	if !(<-goodRef) {
		m.DebugPrint("The prepared transactionblockgroup for mining had bad refs, ignoring\n")
		return nil
	}

	if !(<-goodSign) {
		m.DebugPrint("The prepared transactionblockgroup bad signatures, ignoring\n")
		return nil
	}

	//m.DebugPrint("finished praparation of block\n")

	return newBlock

}

//###############representation#####################

//DebugPrint print if debug is on
func (m *Miner) DebugPrint(msg string) {
	if m != nil {

		m.GeneralMutex.Lock()
		defer m.GeneralMutex.Unlock()

		if m.Debug {
			fmt.Printf("miner %s---- %s", m.Name, msg)
		}
	}
}

// Print print
func (m *Miner) Print() {
	m.GeneralMutex.Lock()
	fmt.Printf("\n-----------------------------\n")
	fmt.Printf("name miner: %s\n", m.Name)
	bc := m.BlockChain
	m.GeneralMutex.Unlock()

	bc.Print()
	fmt.Printf("\n-----------------------------\n")
}

// PrintHash print
func (m *Miner) PrintHash(n int) {
	m.GeneralMutex.Lock()
	fmt.Printf("\n-----------------------------\n")
	k := m.Wallet.PublicKey
	bc := m.BlockChain
	fmt.Printf("name miner: %s (%x)\n", m.Name, k[len(k)-10:])
	m.GeneralMutex.Unlock()

	bc.PrintHash(n)
	fmt.Printf("\n-----------------------------\n")

}

//###########################Networking#################################

// Send broadcast block relevant addresses. Before transmitting the actual data, the receiver is requested to confirm whether he wants the data
func (m *Miner) Send(sendType uint8, data []byte, sender [KeySize]byte, receiver [KeySize]byte) {
	b := m.Broadcaster

	ss := new(SendStruct)
	ss.SendType = sendType
	ss.length = uint64(len(data))
	ss.data = data

	ss.Receiver = receiver
	a := ss.SerializeSendStruct()

	b.LookupMutex.Lock()
	rc, ok := b.Lookup[receiver]
	sd, _ := b.Lookup[sender]
	b.LookupMutex.Unlock()

	h := ss.ConfirmationHash()

	confirmSend := func(c chan []byte) {
		ll := make(chan bool)
		cs := CreateConfirmationStruct(h, sender)

		m.ReceiveChannelMutex.Lock()
		m.ReceiveChannels[cs.response] = ll
		m.ReceiveChannelMutex.Unlock()

		//m.DebugPrint(fmt.Sprintf("Sending %x to %x\n", h[:], ss.Receiver[:]))

		c <- SerializeNetworkMessage(NetworkTypes["Confirmation"], cs.SerializeConfirmationStruct())
		timer1 := time.NewTimer(10 * time.Second)
		select {
		case <-ll:
			c <- SerializeNetworkMessage(NetworkTypes["Send"], a)
		case <-timer1.C:

			//m.DebugPrint(fmt.Sprintf("sending of %x timeout!!!!!!!!!!\n", h[:]))
		}

		m.ReceiveChannelMutex.Lock()
		delete(m.ReceiveChannels, cs.response)
		m.ReceiveChannelMutex.Unlock()

		close(ll)
	}

	if !ok { // receiver unknown or everyone
		var wg sync.WaitGroup

		b.NetworkChannelsMutex.RLock()
		for _, c := range b.NetworkChannels {
			if c != sd {
				wg.Add(1)
				go func(c chan []byte) {
					confirmSend(c)
					wg.Done()
				}(c)

				wg.Wait()

			}
		}
		b.NetworkChannelsMutex.RUnlock()
		return
	}

	confirmSend(rc)
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
	b.NetworkChannelsMutex.RLock()
	for _, c := range b.NetworkChannels {
		c <- SerializeNetworkMessage(NetworkTypes["Request"], a)
	}
	b.NetworkChannelsMutex.RUnlock()

}

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
			go func() {
				cs := DeserializeConfirmationStruct(msg)

				m.ReceivedDataMutex.Lock()
				_, ok := m.ReceivedData[cs.hash]
				m.ReceivedDataMutex.Unlock()

				if !ok {
					val, ok2 := m.Broadcaster.Lookup[cs.sender]
					if ok2 {
						val <- SerializeNetworkMessage(NetworkTypes["ConfirmationAccept"], cs.response[:])
					}
				} else {
					//m.DebugPrint(fmt.Sprintf("Ignoring sendrequest for %x", cs.hash))
				}

			}()

		case NetworkTypes["ConfirmationAccept"]:
			go func() {
				var resp [32]byte
				copy(resp[0:32], msg[0:32])
				m.ReceiveChannelMutex.Lock()
				c, ok := m.ReceiveChannels[resp]
				m.ReceiveChannelMutex.Unlock()
				if ok {
					c <- true
				}
			}()

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

		m.BlockChain.addBlockChainNode(bl)
	case SendType["TransactionBlockGroup"]:
		go m.BlockChain.AddData(ss.data[:])
	case SendType["HeaderAndTransactionBlockGroup"]:
		var a [BlockSize]byte
		copy(a[0:BlockSize], ss.data[0:BlockSize])
		bl := DeserializeBlock(a)

		m.BlockChain.addBlockChainNode(bl)
		go m.BlockChain.AddData(ss.data[:])
	case SendType["Transaction"]:
		fmt.Print("todo implement reception of transaction proof")
	case SendType["Blockchain"]:
		fmt.Printf("receiving of blockchain send not yet implemented")
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
		if bl != nil {
			blData := bl.Block.SerializeBlock()
			m.Send(SendType["BlockHeader"], blData[:], m.Wallet.PublicKey, rq.Requester)
		}

	case RequestType["BlockHeaderFromHeight"]:
		fmt.Printf("todo implement blockheader from height request\n")

	case RequestType["TransactionBlockGroupFromHash"]:

		var hash [32]byte
		copy(hash[0:32], rq.data[0:32])

		bl := m.BlockChain.GetBlockChainNodeAtHash(hash)

		if bl != nil {
			bl.generalMutex.RLock()
			blDataPointer := bl.DataPointer
			bl.generalMutex.RUnlock()

			if blDataPointer != nil {
				blData := blDataPointer.SerializeTransactionBlockGroup()

				//m.DebugPrint(fmt.Sprintf("Received request for data %x, merkle %x , hash %x\n", hash, bl.DataPointer.merkleTree.GetMerkleRoot(), bl.Hash))

				m.Send(SendType["TransactionBlockGroup"], blData[:], m.Wallet.PublicKey, rq.Requester)
			}
		}

	case RequestType["Transaction"]:
		fmt.Printf("todo implement request of transaction")
	}

}

// BroadcastBlock shorthand to broadcast block
func (m *Miner) BroadcastBlock(blc *Block) {
	data := blc.SerializeBlock()
	m.Send(SendType["BlockHeader"], data[:], m.Wallet.PublicKey, Everyone)
}

// RequestBlockFromHash makes request for block
func (m *Miner) RequestBlockFromHash(hash [32]byte) {
	m.Request(RequestType["BlockHeaderFromHash"], m.Wallet.PublicKey, hash[:])
}
