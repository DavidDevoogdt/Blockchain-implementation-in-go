package davidcoin

import (
	"crypto/sha256"
	"fmt"
	rsautil "project/pkg/rsa_util"
	"sync"
	"time"

	"github.com/sasha-s/go-deadlock"
)

// Everyone is shorthand for all subscribed receivers
var Everyone [rsautil.KeySize]byte

// Miner main type
type Miner struct {
	Name       string
	Wallet     *Wallet
	BlockChain *BlockChain

	networkChannel chan []byte
	broadcaster    *Broadcaster
	interrupt      chan bool

	debug    bool
	isMining bool

	generalMutex deadlock.Mutex

	receivedData      map[[32]byte]bool
	receivedDataMutex sync.Mutex

	receiveChannels     map[[32]byte]chan bool
	receiveChannelMutex sync.Mutex

	tp *TransactionPool
}

//###################one time init###################

// CreateMiner initiates miner with given broadcaster and blockchain
func CreateMiner(name string, Broadcaster *Broadcaster, blockChain [][blockSize]byte) *Miner {

	m := new(Miner)
	m.Name = name
	m.Wallet = m.initializeWallet()
	m.BlockChain = DeserializeBlockChain(blockChain)
	m.broadcaster = Broadcaster
	m.networkChannel = make(chan []byte)
	m.receiveChannels = make(map[[32]byte]chan bool)
	m.broadcaster.append(m.networkChannel, m.Wallet.PublicKey)
	m.BlockChain.Miner = m
	m.interrupt = make(chan bool)
	m.debug = false
	m.receivedData = make(map[[32]byte]bool)
	m.BlockChain.Root.uTxOManagerPointer = initializeUTxOMananger(m)
	m.BlockChain.Root.uTxOManagerIsUpToDate = true
	m.tp = m.initializeTransactionPool()

	go m.receiveNetwork()
	go m.BlockChain.utxoUpdater()

	return m
}

// MinerFromScratch ask for own resources for generation of blockchain
func MinerFromScratch(name string, broadcaster *Broadcaster) *Miner {
	m := CreateMiner(name, broadcaster, make([][blockSize]byte, 0))
	//go m.IntRequestBlock(1)
	return m
}

// CreateGenesisMiner called from other place, difficult circular dependencies for first miner
func CreateGenesisMiner(name string, Broadcaster *Broadcaster, blockChain *BlockChain) *Miner {

	m := new(Miner)
	m.Name = name
	m.Wallet = m.initializeWallet()
	m.BlockChain = blockChain
	m.broadcaster = Broadcaster
	m.networkChannel = make(chan []byte)
	m.receiveChannels = make(map[[32]byte]chan bool)
	m.broadcaster.append(m.networkChannel, m.Wallet.PublicKey)
	m.interrupt = make(chan bool)
	m.debug = false
	m.receivedData = make(map[[32]byte]bool)
	m.tp = m.initializeTransactionPool()
	//InitializeUTxOMananger(m)

	go m.receiveNetwork()
	go m.BlockChain.utxoUpdater()
	return m
}

// SetDebug switches logging
func (m *Miner) SetDebug(val bool) {
	m.generalMutex.Lock()
	m.debug = val
	m.generalMutex.Unlock()
}

//###############actual mining #######################""

// mineBlock mines block and add own credentials
func (m *Miner) mineBlock(newBlock *block) *block {
	m.generalMutex.Lock()
	m.isMining = true
	m.generalMutex.Unlock()

	defer func() {
		m.generalMutex.Lock()
		m.isMining = false
		m.generalMutex.Unlock()
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
	for {
		tbg := m.tp.generateTransctionBlockGroup()
		prepBlock := m.tp.prepareBlockForMining(tbg)
		if prepBlock != nil {
			blc := m.mineBlock(prepBlock)
			//print(blc)
			if blc == nil {
				m.DebugPrint(fmt.Sprintf("%s was not fast enough \n", m.Name))
			} else {
				m.DebugPrint(fmt.Sprintf("%s mined block: \n", m.Name))
				m.BlockChain.addInternal(blc, tbg)

				m.broadcastBlock(blc)
			}

		} else {
			m.DebugPrint("Block nil, somthing went wrong with preparation of it\n")
		}
	}
}

//###############representation#####################

//DebugPrint print if debug is on
func (m *Miner) DebugPrint(msg string) {
	if m != nil {

		m.generalMutex.Lock()
		defer m.generalMutex.Unlock()

		if m.debug {
			fmt.Printf("miner %s---- %s", m.Name, msg)
		}
	}
}

// Print print
func (m *Miner) Print() {
	m.generalMutex.Lock()
	fmt.Printf("\n-----------------------------\n")
	fmt.Printf("name miner: %s\n", m.Name)
	bc := m.BlockChain
	m.generalMutex.Unlock()

	bc.Print()
	fmt.Printf("\n-----------------------------\n")
}

// PrintHash print
func (m *Miner) PrintHash(n int) {
	m.generalMutex.Lock()
	fmt.Printf("\n-----------------------------\n")
	k := m.Wallet.PublicKey
	bc := m.BlockChain
	fmt.Printf("name miner: %s (%x)\n", m.Name, k[len(k)-10:])
	m.generalMutex.Unlock()

	bc.PrintHash(n)
	fmt.Printf("\n-----------------------------\n")

}

//###########################Networking#################################

// send broadcast block relevant addresses. Before transmitting the actual data, the receiver is requested to confirm whether he wants the data
func (m *Miner) send(sendType uint8, data []byte, sender [rsautil.KeySize]byte, receiver [rsautil.KeySize]byte) {
	b := m.broadcaster

	ss := new(sendStruct)
	ss.SendType = sendType
	ss.length = uint64(len(data))
	ss.data = data

	ss.Receiver = receiver
	a := ss.SerializeSendStruct()

	b.lookupMutex.Lock()
	rc, ok := b.Lookup[receiver]
	sd, _ := b.Lookup[sender]
	b.lookupMutex.Unlock()

	h := ss.confirmationHash()

	confirmSend := func(c chan []byte) {
		ll := make(chan bool)
		cs := createConfirmationStruct(h, sender)

		m.receiveChannelMutex.Lock()
		m.receiveChannels[cs.response] = ll
		m.receiveChannelMutex.Unlock()

		//m.DebugPrint(fmt.Sprintf("Sending %x to %x\n", h[:], ss.Receiver[:]))

		c <- serializeNetworkMessage(networkTypes["Confirmation"], cs.serializeConfirmationStruct())
		timer1 := time.NewTimer(10 * time.Second)
		select {
		case <-ll:
			c <- serializeNetworkMessage(networkTypes["Send"], a)
		case <-timer1.C:

			//m.DebugPrint(fmt.Sprintf("sending of %x timeout!!!!!!!!!!\n", h[:]))
		}

		m.receiveChannelMutex.Lock()
		delete(m.receiveChannels, cs.response)
		m.receiveChannelMutex.Unlock()

		close(ll)
	}

	if !ok { // receiver unknown or everyone
		var wg sync.WaitGroup

		b.networkChannelsMutex.RLock()
		for _, c := range b.networkChannels {
			if c != sd {
				wg.Add(1)
				go func(c chan []byte) {
					confirmSend(c)
					wg.Done()
				}(c)

				wg.Wait()

			}
		}
		b.networkChannelsMutex.RUnlock()
		return
	}

	confirmSend(rc)
	//rc <- SerializeNetworkMessage(NetworkTypes["Send"], a)
}

// request broadcast a request to everyone
func (m *Miner) request(RequestType uint8, Requester [rsautil.KeySize]byte, data []byte) {
	b := m.broadcaster

	rq := new(requestStruct)
	rq.RequestType = RequestType
	copy(rq.Requester[0:rsautil.KeySize], Requester[0:rsautil.KeySize])
	rq.length = uint64(len(data))
	rq.data = make([]byte, rq.length)
	copy(rq.data[0:rq.length], data[0:rq.length])

	a := rq.serializeRequestStruct()

	//requester := b.Lookup[Requester]
	b.networkChannelsMutex.RLock()
	for _, c := range b.networkChannels {
		c <- serializeNetworkMessage(networkTypes["Request"], a)
	}
	b.networkChannelsMutex.RUnlock()

}

// receiveNetwork decodes all the incoming network traffic. The request are transferred to the coresponding decoders
func (m *Miner) receiveNetwork() {
	for {
		networkType, msg := deserializeNetworkMessage(<-m.networkChannel)
		switch networkType {
		case networkTypes["Send"]:
			go m.receiveSend(msg)
		case networkTypes["Request"]:
			go m.receiveRequest(msg)
		case networkTypes["Confirmation"]:
			go func() {
				cs := deserializeConfirmationStruct(msg)

				m.receivedDataMutex.Lock()
				_, ok := m.receivedData[cs.hash]
				m.receivedDataMutex.Unlock()

				if !ok {
					val, ok2 := m.broadcaster.Lookup[cs.sender]
					if ok2 {
						val <- serializeNetworkMessage(networkTypes["ConfirmationAccept"], cs.response[:])
					}
				} else {
					//m.DebugPrint(fmt.Sprintf("Ignoring sendrequest for %x", cs.hash))
				}

			}()

		case networkTypes["ConfirmationAccept"]:
			go func() {
				var resp [32]byte
				copy(resp[0:32], msg[0:32])
				m.receiveChannelMutex.Lock()
				c, ok := m.receiveChannels[resp]
				m.receiveChannelMutex.Unlock()
				if ok {
					c <- true
				}
			}()

		}

	}
}

// receiveSend decodes the sent data and updates internal structure with the new dat
func (m *Miner) receiveSend(msg []byte) {
	ss := deserializeSendStruct(msg)

	dataHash := sha256.Sum256(ss.data[:])

	m.receivedDataMutex.Lock()
	m.receivedData[dataHash] = true
	m.receivedDataMutex.Unlock()

	switch ss.SendType {
	case sendType["BlockHeader"]:

		var a [blockSize]byte
		copy(a[0:blockSize], ss.data[0:blockSize])
		bl := deserializeBlock(a)

		m.BlockChain.addBlockChainNode(bl)
	case sendType["TransactionBlockGroup"]:
		go m.BlockChain.addData(ss.data[:])
	case sendType["HeaderAndTransactionBlockGroup"]:
		var a [blockSize]byte
		copy(a[0:blockSize], ss.data[0:blockSize])
		bl := deserializeBlock(a)

		m.BlockChain.addBlockChainNode(bl)
		go m.BlockChain.addData(ss.data[:])
	case sendType["Transaction"]:
		m.tp.recieveChannel <- ss.data[:]
		//m.DebugPrint("Received transaction data\n")
	case sendType["Blockchain"]:
		fmt.Printf("receiving of blockchain send not yet implemented")
	}
}

// receiveRequest decodes the request and tries to service the request if possible
func (m *Miner) receiveRequest(msg []byte) {
	rq := deserializeRequestStruct(msg)
	switch rq.RequestType {
	case requestType["BlockHeaderFromHash"]:
		var data [32]byte
		copy(data[0:32], rq.data[0:32])
		bl := m.BlockChain.GetBlockChainNodeAtHash(data)
		if bl != nil {
			blData := bl.Block.serializeBlock()
			m.send(sendType["BlockHeader"], blData[:], m.Wallet.PublicKey, rq.Requester)
		}

	case requestType["BlockHeaderFromHeight"]:
		fmt.Printf("todo implement blockheader from height request\n")

	case requestType["TransactionBlockGroupFromHash"]:

		var hash [32]byte
		copy(hash[0:32], rq.data[0:32])

		bl := m.BlockChain.GetBlockChainNodeAtHash(hash)

		if bl != nil {
			bl.generalMutex.RLock()
			blDataPointer := bl.dataPointer
			bl.generalMutex.RUnlock()

			if blDataPointer != nil {
				blData := blDataPointer.serializeTransactionBlockGroup()

				//m.DebugPrint(fmt.Sprintf("Received request for data %x, merkle %x , hash %x\n", hash, bl.DataPointer.merkleTree.GetMerkleRoot(), bl.Hash))

				m.send(sendType["TransactionBlockGroup"], blData[:], m.Wallet.PublicKey, rq.Requester)
			}
		}

	case requestType["Transaction"]:
		fmt.Printf("todo implement request of transaction")
	}

}

// broadcastBlock shorthand to broadcast block
func (m *Miner) broadcastBlock(blc *block) {
	data := blc.serializeBlock()
	m.send(sendType["BlockHeader"], data[:], m.Wallet.PublicKey, Everyone)
}

// requestBlockFromHash makes request for block
func (m *Miner) requestBlockFromHash(hash [32]byte) {
	m.request(requestType["BlockHeaderFromHash"], m.Wallet.PublicKey, hash[:])
}
