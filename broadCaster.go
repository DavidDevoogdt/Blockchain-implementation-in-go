package main

import (
	"encoding/binary"
	"fmt"
)

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
	"BlockHeader":   0,
	"BlockData":     1,
	"Blockchain":    2,
	"HeaderAndData": 3,
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
	fmt.Printf("data %x\n", rq.data)
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

//########################################

// Broadcaster groups subscribed miners
type Broadcaster struct {
	Lookup          map[[KeySize]byte]chan []byte
	ReceiveChannels []chan []byte
	RequestChannels []chan []byte
	Name            string
	Count           int
}

// NewBroadcaster adds return new subscribed broadcaster
func NewBroadcaster(name string) *Broadcaster {
	bc := new(Broadcaster)
	bc.Name = name
	bc.Count = 0
	bc.ReceiveChannels = make([]chan []byte, 0)
	bc.RequestChannels = make([]chan []byte, 0)

	bc.Lookup = make(map[[KeySize]byte]chan []byte)

	return bc
}

func (b *Broadcaster) append(receive chan []byte, request chan []byte, address [KeySize]byte) {
	b.ReceiveChannels = append(b.ReceiveChannels, receive)
	b.RequestChannels = append(b.RequestChannels, request)
	b.Lookup[address] = receive
	b.Count++
}

// Send broadcast block to everyone
func (b *Broadcaster) Send(sendType uint8, data []byte, sender [KeySize]byte, receiver [KeySize]byte) {

	ss := new(SendStruct)
	ss.SendType = sendType
	ss.length = uint64(len(data))
	ss.data = data
	a := ss.SerializeSendStruct()

	rc, err := b.Lookup[receiver]
	sd, err := b.Lookup[sender]

	if err {
		for _, c := range b.ReceiveChannels {
			if c != sd {
				c <- a
			}

		}
		return
	}

	rc <- a
}

// Request broadcast request to everyone
func (b *Broadcaster) Request(RequestType uint8, Requester [KeySize]byte, data []byte) {
	//fmt.Printf("-------creating request for %x with hash %x", Requester, data)
	rq := new(RequestStruct)
	rq.RequestType = RequestType
	copy(rq.Requester[0:KeySize], Requester[0:KeySize])
	rq.length = uint64(len(data))
	rq.data = make([]byte, rq.length)
	copy(rq.data[0:rq.length], data[0:rq.length])

	a := rq.SerializeRequestStruct()

	//requester := b.Lookup[Requester]
	for _, c := range b.RequestChannels {

		c <- a

	}

}
