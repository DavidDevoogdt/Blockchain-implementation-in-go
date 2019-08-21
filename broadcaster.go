package main

import (
	"crypto/sha256"
	"encoding/binary"
	"math/rand"
)

// NetworkTypes represents categories which can be send over a network
var NetworkTypes = map[string]uint8{
	"Request":            0,
	"Send":               1,
	"Confirmation":       2,
	"ConfirmationAccept": 3,
}

// SerializeNetworkMessage wraps type around message
func SerializeNetworkMessage(networkType uint8, message []byte) []byte {
	return append(message[:], byte(networkType))
}

// DeserializeNetworkMessage wraps type around message
func DeserializeNetworkMessage(networkMessage []byte) (uint8, []byte) {
	length := len(networkMessage)
	return networkMessage[length-1], networkMessage[0 : length-1]
}

//##################################

// RequestType maps intended request to its number
var RequestType = map[string]uint8{
	"BlockHeaderFromHash":   0,
	"BlockHeaderFromHeight": 1,
	"DataFromHash":          2,
	"Transaction":           3,
}

// RequestTypeSize maps numbers to the size of the data
var RequestTypeSize = map[uint8]uint16{ //size of data
	0: KeySize,
	1: 4,
	2: KeySize,
	3: TransactionRequestSize,
}

// SendType maps the int to a human readable type
var SendType = map[string]uint8{
	"BlockHeader":      0,
	"BlockData":        1,
	"Blockchain":       2,
	"HeaderAndData":    3,
	"TransactionBlock": 4,
}

// RequestStruct is dummy type to create request
type RequestStruct struct {
	RequestType uint8
	length      uint64
	Requester   [KeySize]byte
	data        []byte
}

// SerializeRequestStruct serializes request
func (rq *RequestStruct) SerializeRequestStruct() []byte {
	rqSize := rq.length
	ret := make([]byte, 1+8+KeySize+rqSize)
	ret[0] = rq.RequestType
	binary.LittleEndian.PutUint64(ret[1:9], rq.length)
	copy(ret[9:9+KeySize], rq.Requester[0:KeySize])
	copy(ret[9+KeySize:9+KeySize+rqSize], rq.data[0:rqSize])
	return ret
}

// DeserializeRequestStruct does inverse of SerializeRequestStruct
func DeserializeRequestStruct(ret []byte) *RequestStruct {
	rq := new(RequestStruct)
	rq.RequestType = ret[0]
	rq.length = binary.LittleEndian.Uint64(ret[1:9])
	copy(rq.Requester[0:KeySize], ret[9:9+KeySize])
	rq.data = make([]byte, rq.length)

	copy(rq.data[0:rq.length], ret[9+KeySize:9+KeySize+rq.length])
	return rq
}

// SendStruct is wrapper to decode received data
type SendStruct struct {
	SendType uint8
	length   uint64
	Receiver [KeySize]byte // if nil everyone receives
	data     []byte        // has length length
}

// SerializeSendStruct serialize send struct
func (ss *SendStruct) SerializeSendStruct() []byte {
	ret := make([]byte, 1+8+KeySize+ss.length)
	ret[0] = ss.SendType
	binary.LittleEndian.PutUint64(ret[1:9], ss.length)
	copy(ret[9:9+KeySize], ss.Receiver[0:KeySize])
	copy(ret[9+KeySize:9+KeySize+ss.length], ss.data[0:ss.length])
	return ret
}

// DeserializeSendStruct does inverse of serilizesendstruct
func DeserializeSendStruct(ret []byte) *SendStruct {
	ss := new(SendStruct)
	ss.SendType = ret[0]
	ss.length = binary.LittleEndian.Uint64(ret[1:9])
	copy(ss.Receiver[0:KeySize], ret[9:9+KeySize])

	ss.data = make([]byte, ss.length)
	copy(ss.data[0:ss.length], ret[9+KeySize:9+KeySize+ss.length])

	return ss
}

// ConfirmationHash used to check whether receiver wants to receive
func (ss *SendStruct) ConfirmationHash() [32]byte {
	return sha256.Sum256(ss.data)
}

//########################################

// ConfirmationStruct is networktype used for demanding confirmation before sending actual data
type ConfirmationStruct struct {
	hash     [32]byte
	sender   [KeySize]byte
	response [32]byte
}

//CreateConfirmationStruct inits cs
func CreateConfirmationStruct(hash [32]byte, sender [KeySize]byte) *ConfirmationStruct {
	cs := new(ConfirmationStruct)
	cs.hash = hash
	cs.sender = sender
	rand.Read(cs.response[0:32])
	return cs
}

// SerializeConfirmationStruct serializes
func (cs *ConfirmationStruct) SerializeConfirmationStruct() []byte {
	a := make([]byte, 2*32+KeySize)
	copy(a[0:32], cs.hash[0:32])
	copy(a[32:32+KeySize], cs.sender[0:KeySize])
	copy(a[32+KeySize:32*2+KeySize], cs.response[0:32])
	return a
}

// DeserializeConfirmationStruct deserializes
func DeserializeConfirmationStruct(a []byte) *ConfirmationStruct {
	cs := new(ConfirmationStruct)
	copy(cs.hash[0:32], a[0:32])
	copy(cs.sender[0:KeySize], a[32:32+KeySize])
	copy(cs.response[0:32], a[32+KeySize:32*2+KeySize])
	return cs
}

//########################################

// Broadcaster groups subscribed miners
type Broadcaster struct {
	Lookup          map[[KeySize]byte]chan []byte
	NetworkChannels []chan []byte
	Name            string
	Count           int
}

// NewBroadcaster adds return new subscribed broadcaster
func NewBroadcaster(name string) *Broadcaster {
	bc := new(Broadcaster)
	bc.Name = name
	bc.Count = 0
	bc.NetworkChannels = make([]chan []byte, 0)

	bc.Lookup = make(map[[KeySize]byte]chan []byte)

	return bc
}

func (b *Broadcaster) append(network chan []byte, address [KeySize]byte) {
	b.NetworkChannels = append(b.NetworkChannels, network)
	b.Lookup[address] = network
	b.Count++
}
