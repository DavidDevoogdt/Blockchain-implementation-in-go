package davidcoin

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

///capital letter -> exported field

// genesisBlock is first block for everyone
var genesisBlock *block

// block is basic type for node in blockchain
type block struct {
	MerkleRoot [32]byte
	PrevHash   [32]byte
	Nonce      uint32
	Difficulty uint32
	BlockCount uint32
}

// blockSize size of serialized struct
const blockSize = 76

// serializeBlock serialize the content of the block
func (b *block) serializeBlock() [blockSize]byte {
	var d [blockSize]byte
	copy(d[0:32], b.MerkleRoot[0:32])
	copy(d[32:64], b.PrevHash[0:32])
	binary.LittleEndian.PutUint32(d[64:68], b.Nonce)
	binary.LittleEndian.PutUint32(d[68:72], b.Difficulty)
	binary.LittleEndian.PutUint32(d[72:76], b.BlockCount)
	return d
}

// deserializeBlock does the inverse of serializeBlock
func deserializeBlock(d [blockSize]byte) *block {
	b := new(block)
	copy(b.MerkleRoot[0:32], d[0:32])
	copy(b.PrevHash[0:32], d[32:64])
	b.Nonce = binary.LittleEndian.Uint32(d[64:68])
	b.Difficulty = binary.LittleEndian.Uint32(d[68:72])
	b.BlockCount = binary.LittleEndian.Uint32(d[72:76])
	return b
}

// Hash gets block hash
func (b *block) Hash() [32]byte {
	a := b.serializeBlock()
	return sha256.Sum256(a[:])
}

// Print prints human readable description
func (b *block) Print() {
	b.print()
}

// Print prints human readable description
func (b *block) print() {
	fmt.Printf("\n_____________________\n")
	fmt.Printf("num %d\nData: %x \nNonce: %d \nPrevHash %x\ncorrect: %t \nDifficulty %d\nblockHash %x\nLocation %x", b.BlockCount, b.MerkleRoot, b.Nonce, b.PrevHash[:], b.Verify(), b.Difficulty, b.Hash(), &b)
	fmt.Printf("_____________________\n")
}

// Verify checks whether hash is valid according to difficulty
func (b *block) Verify() bool {
	return compHash(b.Difficulty, b.Hash())
}

func compHash(Difficulty uint32, hash [32]byte) bool {
	zero := make([]byte, Difficulty)
	return bytes.Equal(hash[0:Difficulty], zero[:])
}

func generateGenesis(difficulty uint32, m *Miner) *block {
	genesis := new(block)
	genesis.Nonce = 0
	var h [32]byte
	genesis.PrevHash = h

	genesis.MerkleRoot = sha256.Sum256([]byte("hello world!"))
	genesis.Difficulty = difficulty
	genesis.BlockCount = 0
	//copy(genesis.Owner[:], m.Wallet[:])

	for {
		if compHash(genesis.Difficulty, genesis.Hash()) {
			break
		}
		genesis.Nonce++
	}

	genesisBlock = genesis
	return genesis
}
