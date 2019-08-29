package davidcoin
import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

///capital letter -> exported field

// GenesisBlock is first block for everyone
var GenesisBlock *Block

// Block is basic type for node in blockchain
type Block struct {
	MerkleRoot [32]byte
	PrevHash   [32]byte
	Nonce      uint32
	Difficulty uint32
	BlockCount uint32
}

// BlockSize size of serialized struct
const BlockSize = 76

// SerializeBlock serialize the content of the block
func (b *Block) SerializeBlock() [BlockSize]byte {
	var d [BlockSize]byte
	copy(d[0:32], b.MerkleRoot[0:32])
	copy(d[32:64], b.PrevHash[0:32])
	binary.LittleEndian.PutUint32(d[64:68], b.Nonce)
	binary.LittleEndian.PutUint32(d[68:72], b.Difficulty)
	binary.LittleEndian.PutUint32(d[72:76], b.BlockCount)
	return d
}

// DeserializeBlock does the inverse of SerializeBlock
func DeserializeBlock(d [BlockSize]byte) *Block {
	b := new(Block)
	copy(b.MerkleRoot[0:32], d[0:32])
	copy(b.PrevHash[0:32], d[32:64])
	b.Nonce = binary.LittleEndian.Uint32(d[64:68])
	b.Difficulty = binary.LittleEndian.Uint32(d[68:72])
	b.BlockCount = binary.LittleEndian.Uint32(d[72:76])
	return b
}

// Hash gets block hash
func (b *Block) Hash() [32]byte {
	a := b.SerializeBlock()
	return sha256.Sum256(a[:])
}

// Print prints human readable description
func (b *Block) Print() {
	b.print()
}

// Print prints human readable description
func (b *Block) print() {
	fmt.Printf("\n_____________________\n")
	fmt.Printf("num %d\nData: %x \nNonce: %d \nPrevHash %x\ncorrect: %t \nDifficulty %d\nblockHash %x\nLocation %x", b.BlockCount, b.MerkleRoot, b.Nonce, b.PrevHash[:], b.Verify(), b.Difficulty, b.Hash(), &b)
	fmt.Printf("_____________________\n")
}

// Verify checks whether hash is valid according to difficulty
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

	GenesisBlock = genesis
	return genesis
}