package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	mathr "math/rand"
	"project/pkg/merkletree"
	"testing"
)

func TestMerkletree(t *testing.T) {

	mt0 := merkletree.InitializeMerkleTree()
	token := make([]byte, 4)
	rand.Read(token)
	mt0.Add(&token)
	mt0.FinalizeTree()

	hash := sha256.Sum256(token)
	mr := mt0.GetMerkleRoot()

	if !bytes.Equal(mr[0:32], hash[0:32]) {
		t.Errorf("merkleroot for single element does not match its hash\n")
	}

	mp := mt0.GenerareteMerkleProof(0)

	if !mp.VerifyProofStruct(hash) {
		t.Errorf("generated merkleproof wrong for %d \n", 0)
	}

	mp2 := merkletree.DeserializeProofStruct(mp.SerializeProofStruct())
	if !mp2.VerifyProofStruct(hash) {
		t.Errorf("ser/ deser went wrong for %d \n", 0)
	}

	for j := 1; j < 255; j = j + 1 {
		mt := merkletree.InitializeMerkleTree()
		for i := 0; i < j; i++ {
			token := make([]byte, 4)
			rand.Read(token)
			mt.Add(&token)
		}
		mt.FinalizeTree()

		hash := mt.GetMerkleRoot()

		//rand num between 0 and j

		r := uint8(mathr.Float64()*float64(j)) % uint8(j)
		fmt.Printf("num of elem: %d , indexnum :%d\n", j, r)

		mp := mt.GenerareteMerkleProof(r)
		if !mp.VerifyProofStruct(hash) {
			t.Errorf("generated merkleproof wrong for %d \n", j)
		}

		mp2 := merkletree.DeserializeProofStruct(mp.SerializeProofStruct())
		if !mp2.VerifyProofStruct(hash) {
			t.Errorf("ser/ deser went wrong for %d \n", j)
		}

	}

}
