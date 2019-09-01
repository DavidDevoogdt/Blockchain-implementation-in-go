package davidcoin

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand"
	rsautil "project/pkg/rsa_util"
	"sync"
)

// networkTypes represents categories which can be send over a network
var networkTypes = map[string]uint8{
	"Request":            0,
	"Send":               1,
	"Confirmation":       2,
	"ConfirmationAccept": 3,
}

// serializeNetworkMessage wraps type around message
func serializeNetworkMessage(networkType uint8, message []byte) []byte {
	return append(message[:], byte(networkType))
}

// deserializeNetworkMessage wraps type around message
func deserializeNetworkMessage(networkMessage []byte) (uint8, []byte) {
	length := len(networkMessage)
	return networkMessage[length-1], networkMessage[0 : length-1]
}

//##################################

// requestType maps intended request to its number
var requestType = map[string]uint8{
	"BlockHeaderFromHash":           0,
	"BlockHeaderFromHeight":         1,
	"TransactionBlockGroupFromHash": 2,
	"Transaction":                   3,
}

// requestTypeSize maps numbers to the size of the data
var requestTypeSize = map[uint8]uint16{ //size of data
	0: rsautil.KeySize,
	1: 4,
	2: rsautil.KeySize,
}

// sendType maps the int to a human readable type
var sendType = map[string]uint8{
	"BlockHeader":                    0,
	"TransactionBlockGroup":          1,
	"Blockchain":                     2,
	"HeaderAndTransactionBlockGroup": 3,
	"Transaction":                    4,
}

// requestStruct is dummy type to create request
type requestStruct struct {
	RequestType uint8
	length      uint64
	Requester   [rsautil.KeySize]byte
	data        []byte
}

// serializeRequestStruct serializes request
func (rq *requestStruct) serializeRequestStruct() []byte {
	rqSize := rq.length
	ret := make([]byte, 1+8+rsautil.KeySize+rqSize)
	ret[0] = rq.RequestType
	binary.LittleEndian.PutUint64(ret[1:9], rq.length)
	copy(ret[9:9+rsautil.KeySize], rq.Requester[0:rsautil.KeySize])
	copy(ret[9+rsautil.KeySize:9+rsautil.KeySize+rqSize], rq.data[0:rqSize])
	return ret
}

// deserializeRequestStruct does inverse of SerializeRequestStruct
func deserializeRequestStruct(ret []byte) *requestStruct {
	rq := new(requestStruct)
	rq.RequestType = ret[0]
	rq.length = binary.LittleEndian.Uint64(ret[1:9])
	copy(rq.Requester[0:rsautil.KeySize], ret[9:9+rsautil.KeySize])
	rq.data = make([]byte, rq.length)

	copy(rq.data[0:rq.length], ret[9+rsautil.KeySize:9+rsautil.KeySize+rq.length])
	return rq
}

// sendStruct is wrapper to decode received data
type sendStruct struct {
	SendType uint8
	length   uint64
	Receiver [rsautil.KeySize]byte // if nil everyone receives
	data     []byte                // has length length
}

// SerializeSendStruct serialize send struct
func (ss *sendStruct) SerializeSendStruct() []byte {
	ret := make([]byte, 1+8+rsautil.KeySize+ss.length)
	ret[0] = ss.SendType
	binary.LittleEndian.PutUint64(ret[1:9], ss.length)
	copy(ret[9:9+rsautil.KeySize], ss.Receiver[0:rsautil.KeySize])
	copy(ret[9+rsautil.KeySize:9+rsautil.KeySize+ss.length], ss.data[0:ss.length])
	return ret
}

// deserializeSendStruct does inverse of serilizesendstruct
func deserializeSendStruct(ret []byte) *sendStruct {
	ss := new(sendStruct)
	ss.SendType = ret[0]
	ss.length = binary.LittleEndian.Uint64(ret[1:9])
	copy(ss.Receiver[0:rsautil.KeySize], ret[9:9+rsautil.KeySize])

	ss.data = make([]byte, ss.length)
	copy(ss.data[0:ss.length], ret[9+rsautil.KeySize:9+rsautil.KeySize+ss.length])

	return ss
}

// confirmationHash used to check whether receiver wants to receive
func (ss *sendStruct) confirmationHash() [32]byte {
	return sha256.Sum256(ss.data)
}

//########################################

// confirmationStruct is networktype used for demanding confirmation before sending actual data
type confirmationStruct struct {
	hash     [32]byte
	sender   [rsautil.KeySize]byte
	response [32]byte
}

//createConfirmationStruct inits cs
func createConfirmationStruct(hash [32]byte, sender [rsautil.KeySize]byte) *confirmationStruct {
	cs := new(confirmationStruct)
	cs.hash = hash
	cs.sender = sender
	rand.Read(cs.response[0:32])
	return cs
}

// serializeConfirmationStruct serializes
func (cs *confirmationStruct) serializeConfirmationStruct() []byte {
	a := make([]byte, 2*32+rsautil.KeySize)
	copy(a[0:32], cs.hash[0:32])
	copy(a[32:32+rsautil.KeySize], cs.sender[0:rsautil.KeySize])
	copy(a[32+rsautil.KeySize:32*2+rsautil.KeySize], cs.response[0:32])
	return a
}

// deserializeConfirmationStruct deserializes
func deserializeConfirmationStruct(a []byte) *confirmationStruct {
	cs := new(confirmationStruct)
	copy(cs.hash[0:32], a[0:32])
	copy(cs.sender[0:rsautil.KeySize], a[32:32+rsautil.KeySize])
	copy(cs.response[0:32], a[32+rsautil.KeySize:32*2+rsautil.KeySize])
	return cs
}

//########################################

// Broadcaster groups subscribed miners
type Broadcaster struct {
	lookupMutex          sync.Mutex
	Lookup               map[[rsautil.KeySize]byte]chan []byte
	networkChannels      []chan []byte
	networkChannelsMutex sync.RWMutex
	Name                 string
	Count                int
}

// NewBroadcaster adds return new subscribed broadcaster
func NewBroadcaster(name string) *Broadcaster {
	bc := new(Broadcaster)
	bc.Name = name
	bc.Count = 0
	bc.networkChannels = make([]chan []byte, 0)

	bc.Lookup = make(map[[rsautil.KeySize]byte]chan []byte)

	return bc
}

func (b *Broadcaster) append(network chan []byte, address [rsautil.KeySize]byte) {
	b.networkChannelsMutex.Lock()
	b.networkChannels = append(b.networkChannels, network)
	b.networkChannelsMutex.Unlock()
	b.lookupMutex.Lock()
	b.Lookup[address] = network
	b.lookupMutex.Unlock()
	b.Count++
}
