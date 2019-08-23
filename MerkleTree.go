package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/bits"
)

// ProofStruct is datatype containing the serialized data and the hashes to verify the data
type ProofStruct struct {
	applicationOrder uint8 // for each zero , the hash should be joined left with a hash from the proofarray, otherwise right
	proofSize        uint8
	proofArray       []byte
	data             []byte
}

// SerializeProofStruct makes it ready for network transport
func (ps *ProofStruct) SerializeProofStruct() []byte {
	dlen := len(ps.data)
	psize := int(ps.proofSize) * 32
	ret := make([]byte, 1+1+psize+dlen)
	ret[0] = ps.applicationOrder
	ret[1] = ps.proofSize
	copy(ret[2:2+psize], ps.proofArray[0:psize])
	copy(ret[2+psize:2+psize+dlen], ps.data[0:dlen])
	return ret
}

// DeserializeProofStruct turns byte array back into proofstruct
func DeserializeProofStruct(ret []byte) *ProofStruct {
	pf := new(ProofStruct)

	pf.applicationOrder = ret[0]
	pf.proofSize = ret[1]
	psize := int(pf.proofSize) * 32
	pf.proofArray = make([]byte, psize)
	copy(pf.proofArray[0:psize], ret[2:2+psize])
	dlen := len(ret) - psize - 2
	pf.data = make([]byte, dlen)
	copy(pf.data[0:dlen], ret[2+psize:2+psize+dlen])
	return pf
}

// VerifyProofStruct checks whether data is correct
func (ps *ProofStruct) VerifyProofStruct(expected [32]byte) bool {
	temp := sha256.Sum256(ps.data[:])

	var buf [64]byte
	for i := uint8(0); i < ps.proofSize; i++ {
		if (ps.applicationOrder>>i)&1 == 1 {
			copy(buf[0:32], ps.proofArray[i*32:(i+1)*32])
			copy(buf[32:64], temp[0:32])
		} else {
			copy(buf[0:32], temp[0:32])
			copy(buf[32:64], ps.proofArray[i*32:(int(i)+1)*32])
		}
		temp = sha256.Sum256(buf[:])
	}

	return bytes.Equal(temp[0:32], expected[0:32])
}

// MerkleTree keeps track of the elements.
type MerkleTree struct {
	elements        uint8
	data            []*[]byte
	levelHash       [][][32]byte // an array with 2**i elements for each level i. Each element contains the hash of the previous 2 levels
	finalized       bool
	lowerLevel      uint8
	bitsOnHighLevel uint8
}

// GetMerkleRoot returns top hash
func (mt *MerkleTree) GetMerkleRoot() [32]byte {
	var ret [32]byte
	if !mt.finalized {
		fmt.Printf("tree not yet finalized")
		return ret
	}
	copy(ret[0:32], mt.levelHash[0][0][0:32])
	return ret
}

// Add simply adds one elem
func (mt *MerkleTree) Add(a *[]byte) {
	if mt.finalized {
		fmt.Printf("tree already finalize, not added")
	} else {
		if mt.elements <= 254 {

			mt.elements++
			mt.data = append(mt.data, a)
		} else {
			fmt.Printf("to many elements, not added\n")
		}
	}

}

// InitializeMerkleTree creates empty tree
func InitializeMerkleTree() *MerkleTree {
	mt := new(MerkleTree)
	mt.elements = 0
	mt.data = make([]*[]byte, 0)
	mt.finalized = false
	return mt
}

// FinalizeTree takes all elements and generates the tree structure and the hashes
func (mt *MerkleTree) FinalizeTree() {
	mt.lowerLevel = uint8(bits.Len8(mt.elements)) - 1
	mt.bitsOnHighLevel = 2 * (mt.elements - 1<<mt.lowerLevel)

	// fill highest complete filled level
	if mt.bitsOnHighLevel != 0 {
		mt.levelHash = make([][][32]byte, mt.lowerLevel+2)

		mt.levelHash[mt.lowerLevel+1] = make([][32]byte, mt.bitsOnHighLevel)
		// first fill the highes level
		for i := uint8(0); i < mt.bitsOnHighLevel; i++ {
			mt.levelHash[mt.lowerLevel+1][i] = sha256.Sum256((*mt.data[i])[:])
		}
		var buf [64]byte
		// fill the first full level, either with data hash or with hash of previous 2 hashes
		mt.levelHash[mt.lowerLevel] = make([][32]byte, 1<<mt.lowerLevel)

		for i := uint8(0); i < (1 << mt.lowerLevel); i++ {
			if i < (mt.bitsOnHighLevel >> 1) {
				copy(buf[0:32], mt.levelHash[mt.lowerLevel+1][i<<1][0:32])
				copy(buf[32:64], mt.levelHash[mt.lowerLevel+1][i<<1+1][0:32])
				mt.levelHash[mt.lowerLevel][i] = sha256.Sum256(buf[:])
			} else {
				mt.levelHash[mt.lowerLevel][i] = sha256.Sum256((*mt.data[i+mt.bitsOnHighLevel/2])[:])
			}

		}
	} else {
		mt.levelHash = make([][][32]byte, mt.lowerLevel+1)
		mt.levelHash[mt.lowerLevel] = make([][32]byte, 1<<mt.lowerLevel)
		for i := uint8(0); i < (1 << mt.lowerLevel); i++ {
			mt.levelHash[mt.lowerLevel][i] = sha256.Sum256((*mt.data[i])[:])
		}

	}

	for i := mt.lowerLevel - 1; i != 255; i-- {
		mt.levelHash[i] = make([][32]byte, 1<<i)

		var buf [64]byte
		// fill the first full level, either with data hash or with hash of previous 2 hashes
		for j := uint8(0); j < (1 << i); j++ {
			copy(buf[0:32], mt.levelHash[i+1][j<<1][0:32])
			copy(buf[32:64], mt.levelHash[i+1][j<<1+1][0:32])
			mt.levelHash[i][j] = sha256.Sum256(buf[:])
		}
	}
	mt.finalized = true
}

// GenerareteMerkleProof takes element and assembles list of hashes for proof
func (mt *MerkleTree) GenerareteMerkleProof(elementNum uint8) *ProofStruct {
	if !mt.finalized {
		fmt.Printf("merkle tree not finalized yet, returning")
		return nil
	}
	if elementNum >= mt.elements {
		fmt.Printf("Elementnum larger than the number of stored elemenets, quiting")
		return nil
	}

	ps := new(ProofStruct)
	ps.data = *mt.data[elementNum]

	if elementNum < mt.bitsOnHighLevel {
		ps.applicationOrder = elementNum
		ps.proofSize = mt.lowerLevel + 1
	} else {
		ps.applicationOrder = elementNum - mt.bitsOnHighLevel/2
		ps.proofSize = mt.lowerLevel
	}

	ps.proofArray = make([]byte, 32*int(ps.proofSize))
	k := ps.applicationOrder

	for i := 0; i <= int(ps.proofSize-1); i++ {
		layerIndex := int(ps.proofSize) - i

		b := mt.levelHash[layerIndex][k^1]
		copy(ps.proofArray[i*32:(i+1)*32], mt.levelHash[layerIndex][k^1][0:32]) // k xor 1 gives the hash next to
		k >>= 1                                                                 // shift goes up one layer

		_ = b
	}

	return ps
}
