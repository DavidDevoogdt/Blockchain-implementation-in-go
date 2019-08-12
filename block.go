package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

///capital letter -> exported field

// BlockSize size of serialized struct
const BlockSize = 168

// GenesisBlock is first block for everyone
var GenesisBlock *Block

// Block is basic type for node in blockchain
type Block struct {
	Data       [32]byte
	PrevHash   [32]byte
	Nonce      uint32
	Difficulty uint32
	BlockCount uint32
	Owner      [8]byte
}

// 5*32+8 = 168

func (b *Block) preHash() [BlockSize]byte {
	var d [168]byte
	copy(d[0:32], b.Data[0:32])
	copy(d[32:64], b.PrevHash[0:32])
	binary.LittleEndian.PutUint32(d[64:96], b.Nonce)
	binary.LittleEndian.PutUint32(d[96:128], b.Difficulty)
	binary.LittleEndian.PutUint32(d[128:160], b.BlockCount)
	copy(d[160:168], b.Owner[0:8])
	return d
}

func (b *Block) serialize() [BlockSize]byte {

	return b.preHash()
}

func deSerialize(d [BlockSize]byte) *Block {
	b := new(Block)
	copy(b.Data[0:32], d[0:32])
	copy(b.PrevHash[0:32], d[32:64])
	b.Nonce = binary.LittleEndian.Uint32(d[64:96])
	b.Difficulty = binary.LittleEndian.Uint32(d[96:128])
	b.BlockCount = binary.LittleEndian.Uint32(d[128:160])
	copy(b.Owner[0:8], d[160:168])

	return b
}

// Hash gets block hash
func (b *Block) Hash() [32]byte {
	a := b.preHash()
	return sha256.Sum256(a[:])
}

// Print prints human readable discription
func (b *Block) Print() {
	fmt.Printf("\n_____________________\n")
	fmt.Printf("num %d\nData: %x \nNonce: %d \nPrevHash %x\ncorrect: %t \nDifficulty %d\nblockHash %x\nOwner %x\nLocation %x", b.BlockCount, b.Data, b.Nonce, b.PrevHash[:], b.Verify(), b.Difficulty, b.Hash(), b.Owner[:], &b)
	fmt.Printf("_____________________\n")
}

// Verify checks wheter hash is valid according to difficulty
func (b *Block) Verify() bool {
	return compHash(b.Difficulty, b.Hash())
}

func compHash(Difficulty uint32, hash [32]byte) bool {
	zero := make([]byte, Difficulty)
	return bytes.Equal(hash[0:Difficulty], zero[:])
}

func generateGenesis(difficulty uint32, m *Miner) *Block {
	genesis := new(Block)
	genesis.Nonce = 0
	var h [32]byte
	genesis.PrevHash = h

	genesis.Data = sha256.Sum256([]byte("hello world!"))
	genesis.Difficulty = difficulty
	genesis.BlockCount = 0
	copy(genesis.Owner[:], m.Wallet[:])

	for {
		if compHash(genesis.Difficulty, genesis.Hash()) {
			break
		}
		genesis.Nonce++
	}

	GenesisBlock = genesis
	return genesis
}
