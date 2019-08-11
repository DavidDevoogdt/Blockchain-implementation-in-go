package main

// IntRequest is channel to request block by height in stack
type IntRequest struct {
	Height    uint32
	requester chan [BlockSize]byte
}

// NewIntRequest creates new int request
func NewIntRequest(height uint32, requester chan [BlockSize]byte) IntRequest {
	return IntRequest{Height: height, requester: requester}
}

// HashRequest is channel to request full block by hash
type HashRequest struct {
	Hash      [32]byte
	requester chan []byte
}

// Broadcaster groups subscribed miners
type Broadcaster struct {
	ReceiveChannels     []chan [BlockSize]byte
	IntRequestChannels  []chan IntRequest
	HashRequestChannels []chan HashRequest
	Name                string
	Count               int
}

// NewBroadcaster adds return new subscribed broadcaster
func NewBroadcaster(name string) *Broadcaster {
	bc := new(Broadcaster)
	bc.Name = name
	bc.Count = 0
	bc.ReceiveChannels = make([]chan [BlockSize]byte, 0)
	bc.IntRequestChannels = make([]chan IntRequest, 0)
	bc.HashRequestChannels = make([]chan HashRequest, 0)

	return bc
}

func (b *Broadcaster) append(receive chan [BlockSize]byte, intRequest chan IntRequest, hashRequest chan HashRequest) {
	b.ReceiveChannels = append(b.ReceiveChannels, receive)
	b.IntRequestChannels = append(b.IntRequestChannels, intRequest)
	b.HashRequestChannels = append(b.HashRequestChannels, hashRequest)
	b.Count++
}

// SendBlock broadcast block to everyone
func (b *Broadcaster) SendBlock(a [BlockSize]byte, sender chan [BlockSize]byte) {
	for _, c := range b.ReceiveChannels {
		if c != sender {
			c <- a
		}

	}
}

// SendBlockTo broadcast block to specific channel
func (b *Broadcaster) SendBlockTo(a []byte, receiver chan []byte) {
	receiver <- a

}

// IntRequestBlock request block by int to everyone
func (b *Broadcaster) IntRequestBlock(a IntRequest) {
	for _, c := range b.IntRequestChannels {
		c <- a
	}
}

// IntRequestBlock request block by hash to everyone
func (b *Broadcaster) hashRequestBlock(a HashRequest) {
	for _, c := range b.HashRequestChannels {
		c <- a
	}
}
