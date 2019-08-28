package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const outputTextSize = 32

// TransactionRefSize is size of a reference tor a transaction in the blockchain
const TransactionRefSize = 34

// TransactionInputSize is len of a transaction input
const TransactionInputSize = SignatureSize + TransactionRefSize

// TransactionOutputSize is len of a transaction output
const TransactionOutputSize = 8 + SignatureSize + KeySize + 32 + outputTextSize

// TransactionRef dd
type TransactionRef struct {
	BlockHash              [32]byte
	TransactionBlockNumber uint8
	OutputNumber           uint8
}

// SerializeTransactionRef serializes transaction
func (tr *TransactionRef) SerializeTransactionRef() [TransactionRefSize]byte {
	var ret [TransactionRefSize]byte
	copy(ret[0:32], tr.BlockHash[0:32])
	ret[32] = tr.OutputNumber
	ret[33] = tr.TransactionBlockNumber
	return ret
}

// DeserializeTransactionRef deserializes transaction
func DeserializeTransactionRef(ret [TransactionRefSize]byte) *TransactionRef {
	tr := new(TransactionRef)
	copy(tr.BlockHash[0:32], ret[0:32])
	tr.OutputNumber = ret[32]
	tr.TransactionBlockNumber = ret[33]
	return tr
}

//####################################

// TransactionBlockNode keeps usefull information for serialization
type TransactionBlockNode struct {
	Hash             [32]byte
	Length           uint64
	TransactionBlock *TransactionBlock
}

//SerializeTransactionBlockNode makes transaction block ready for transmission
func SerializeTransactionBlockNode(tb *TransactionBlock, Parent [32]byte) []byte {
	tbn := new(TransactionBlockNode)
	ser := tb.SerializeTransactionBlock()
	tbn.Length = uint64(len(ser))
	tbn.Hash = tb.Hash()
	ret := make([]byte, 32+8+tbn.Length)
	copy(ret[0:32], tbn.Hash[0:32])
	//copy(ret[32:64], tbn.Parent[0:32])
	binary.LittleEndian.PutUint64(ret[32:40], tbn.Length)
	copy(ret[40:40+tbn.Length], ser[:])
	return ret
}

//DeserializeTransactionBlockNode makes transaction block ready for transmission. Blockchain is used to recover pointer to actual data, not reference
func DeserializeTransactionBlockNode(ret []byte, bc *BlockChain) *TransactionBlockNode {
	tbn := new(TransactionBlockNode)
	copy(tbn.Hash[0:32], ret[0:32])
	tbn.Length = binary.LittleEndian.Uint64(ret[32:40])
	tbn.TransactionBlock = DeserializeTransactionBlock(ret[40:40+tbn.Length], bc)
	return tbn
}

//####################################

// TransactionBlock groups transactions
type TransactionBlock struct {
	InputNumber    uint8
	OutputNumber   uint8
	InputList      []*TransactionInput
	OutputList     []*TransactionOutput
	Signature      [SignatureSize]byte
	PayerPublicKey [KeySize]byte
}

// InitializeTransactionBlock setups basic stuff in transactionblock
func InitializeTransactionBlock() *TransactionBlock {
	tb := new(TransactionBlock)
	tb.InputNumber = 0
	tb.OutputNumber = 0
	tb.InputList = make([]*TransactionInput, 0)
	tb.OutputList = make([]*TransactionOutput, 0)
	return tb
}

// AddOutput ads outp to list
func (tb *TransactionBlock) AddOutput(to *TransactionOutput) {
	tb.OutputList = append(tb.OutputList, to)
	tb.OutputNumber++
}

// SerializeTransactionBlock serializes transactionblock
func (tb *TransactionBlock) SerializeTransactionBlock() []byte {

	buf := make([]byte, int(tb.InputNumber)*TransactionInputSize+int(tb.OutputNumber)*TransactionOutputSize+2+SignatureSize+KeySize)
	buf[0] = tb.InputNumber
	buf[1] = tb.OutputNumber
	ind := 2
	for i := 0; i < int(tb.InputNumber); i++ {
		a := tb.InputList[i].SerializeTransactionInput()
		copy(buf[ind+i*TransactionInputSize:ind+(i+1)*TransactionInputSize], a[0:TransactionInputSize])
	}
	ind = int(tb.InputNumber)*TransactionInputSize + 2
	for i := 0; i < int(tb.OutputNumber); i++ {
		a := tb.OutputList[i].SerializeTransactionOutput()
		copy(buf[ind+i*TransactionOutputSize:ind+(i+1)*TransactionOutputSize], a[0:TransactionOutputSize])
	}

	ind = int(tb.InputNumber)*TransactionInputSize + int(tb.OutputNumber)*TransactionOutputSize + 2

	copy(buf[ind:ind+KeySize], tb.PayerPublicKey[0:KeySize])
	ind += KeySize
	copy(buf[ind:ind+SignatureSize], tb.Signature[0:SignatureSize])

	return buf[:]
}

// DeserializeTransactionBlock does the inverse of serialize. The Blockchain is used to recover pointer to dataoutput instead of reference struct
func DeserializeTransactionBlock(buf []byte, bc *BlockChain) *TransactionBlock {
	tb := new(TransactionBlock)
	inputNumber := uint8(buf[0])
	outputNumber := uint8(buf[1])
	tb.InputNumber = inputNumber
	tb.OutputNumber = outputNumber
	tb.InputList = make([]*TransactionInput, inputNumber)
	tb.OutputList = make([]*TransactionOutput, outputNumber)
	ind := 2
	for i := 0; i < int(tb.InputNumber); i++ {
		var b [TransactionInputSize]byte
		copy(b[0:TransactionInputSize], buf[ind+i*TransactionInputSize:ind+(i+1)*TransactionInputSize])
		tb.InputList[i] = DeserializeTransactionInput(b, bc)
	}
	ind = 2 + int(tb.InputNumber)*TransactionInputSize

	for i := 0; i < int(tb.OutputNumber); i++ {
		var b [TransactionOutputSize]byte
		copy(b[0:TransactionOutputSize], buf[ind+i*TransactionOutputSize:ind+(i+1)*TransactionOutputSize])

		tb.OutputList[i] = DeserializeTransactionOutput(b)
	}
	ind = int(tb.InputNumber)*TransactionInputSize + int(tb.OutputNumber)*TransactionOutputSize + 2
	copy(tb.PayerPublicKey[0:KeySize], buf[ind:ind+KeySize])

	ind += KeySize
	copy(tb.Signature[0:SignatureSize], buf[ind:ind+SignatureSize])

	return tb
}

// GetPreSignature fetches the hash of the block without the signature
func (tb *TransactionBlock) GetPreSignature() [32]byte {
	ser := tb.SerializeTransactionBlock()
	var temp [32]byte
	hash := sha256.Sum256(ser[0 : len(ser)-SignatureSize])
	copy(temp[0:32], hash[0:32])
	return temp
}

//SignBlock adds the signature to the block given a private key
func (tb *TransactionBlock) SignBlock(priv *rsa.PrivateKey) {
	pre := tb.GetPreSignature()
	signature := (SignWithPrivateKey(pre, priv))
	copy(tb.Signature[0:SignatureSize], signature[0:SignatureSize])
}

// VerifyExceptUTXO is necessary to check whether the coins really belong to the spender, check signatures of block and make sure output is not to large
func (tb *TransactionBlock) VerifyExceptUTXO(feeBlock bool) bool {
	//fmt.Printf("------------------>todo verify input signatures<------------------\n")

	if !VerifyWithPublicKey(tb.GetPreSignature(), tb.Signature, BytesToPublicKey(tb.PayerPublicKey)) {
		fmt.Printf("txblock master signature was not good")
		return false
	}
	for _, inp := range tb.InputList {
		if !inp.Verify() {
			fmt.Printf("one of the inputs signatures was not good")
			return false
		}
	}

	inputAmount := uint64(0)

	for _, inp := range tb.InputList {
		if !bytes.Equal(inp.OutputBlock.ReceiverPublicKey[:], tb.PayerPublicKey[:]) {
			fmt.Printf("theres a transactioninput which does not belong to the block owner, returning")
			return false
		}
		inputAmount += inp.OutputBlock.Amount
	}

	outputAmount := uint64(0)

	for _, outp := range tb.OutputList {
		outputAmount += outp.Amount
	}

	if feeBlock {
		return outputAmount <= inputAmount+1*davidcoin

	}
	return outputAmount <= inputAmount

}

// Hash generates hash of serialized transactionBlock
func (tb *TransactionBlock) Hash() [32]byte {
	b := tb.SerializeTransactionBlock()
	return sha256.Sum256(b[:])

}

//##############################################

// TransactionInput is a input transaction (reference to previous output and proof )
type TransactionInput struct {
	VerificationChallenge [SignatureSize]byte // signed by receiver
	OutputBlock           *TransactionOutput  //not serialized
	reference             *TransactionRef     // where this block is stored in chain
}

// SerializeTransactionInput puts transaction into byte array
func (tx *TransactionInput) SerializeTransactionInput() [TransactionInputSize]byte {
	var d [TransactionInputSize]byte
	copy(d[0:SignatureSize], tx.VerificationChallenge[0:SignatureSize])
	temp := tx.reference.SerializeTransactionRef()
	copy(d[SignatureSize:SignatureSize+TransactionRefSize], temp[0:TransactionRefSize])
	return d
}

// DeserializeTransactionInput turns it back into struct. blockchain is used to recover pointer to data
func DeserializeTransactionInput(d [TransactionInputSize]byte, bc *BlockChain) *TransactionInput {
	tx := new(TransactionInput)
	copy(tx.VerificationChallenge[0:SignatureSize], d[0:SignatureSize])
	var o [TransactionRefSize]byte
	copy(o[0:TransactionRefSize], d[SignatureSize:SignatureSize+TransactionRefSize])
	tx.reference = DeserializeTransactionRef(o)
	tx.OutputBlock = bc.GetTransactionOutput(tx.reference)
	return tx
}

// Hash generates hash of serialized transactionBlock
func (tx *TransactionInput) Hash() [32]byte {
	b := tx.SerializeTransactionInput()
	return sha256.Sum256(b[0 : TransactionInputSize-32])
}

// Verify verify ownership of payer
func (tx *TransactionInput) Verify() bool {
	return VerifyWithPublicKey(tx.Hash(), tx.VerificationChallenge, BytesToPublicKey(tx.OutputBlock.ReceiverPublicKey))
}

//##############################################

// TransactionOutput is a Output or output transaction
type TransactionOutput struct {
	Amount            uint64
	Signature         [SignatureSize]byte //payer signature
	ReceiverPublicKey [KeySize]byte
	text              [outputTextSize]byte
}

// SerializeTransactionOutput puts transaction into byte array
func (tx *TransactionOutput) SerializeTransactionOutput() [TransactionOutputSize]byte {
	var d [TransactionOutputSize]byte
	binary.LittleEndian.PutUint64(d[0:8], tx.Amount)
	copy(d[8:8+KeySize], tx.ReceiverPublicKey[0:KeySize])
	ind := 8 + KeySize
	copy(d[ind:ind+outputTextSize], tx.text[0:outputTextSize])
	ind += outputTextSize
	copy(d[ind:ind+SignatureSize], tx.Signature[0:SignatureSize])

	return d
}

// DeserializeTransactionOutput turns it back into struct
func DeserializeTransactionOutput(d [TransactionOutputSize]byte) *TransactionOutput {
	tx := new(TransactionOutput)
	tx.Amount = binary.LittleEndian.Uint64(d[0:8])
	copy(tx.ReceiverPublicKey[0:KeySize], d[8:8+KeySize])
	ind := 8 + KeySize
	copy(tx.text[0:outputTextSize], d[ind:ind+outputTextSize])
	ind += outputTextSize
	copy(tx.Signature[0:SignatureSize], d[ind:ind+SignatureSize])
	return tx
}

// PresSignatureHash generates hash of serialized transactionBlock
func (tx *TransactionOutput) PresSignatureHash() [32]byte {
	b := tx.SerializeTransactionOutput()
	return sha256.Sum256(b[0 : TransactionOutputSize-SignatureSize])
}

// Sign is used to proof the payer ordered the spending
func (tx *TransactionOutput) Sign(priv *rsa.PrivateKey) {
	signature := SignWithPrivateKey(tx.PresSignatureHash(), priv)
	copy(tx.Signature[0:SignatureSize], signature[0:SignatureSize])
}

// Verify verify ownership of payer
func (tx *TransactionOutput) Verify(PayerPublicKey [KeySize]byte) bool {
	return VerifyWithPublicKey(tx.PresSignatureHash(), tx.Signature, BytesToPublicKey(PayerPublicKey))
}

//###########################################################

// TransactionBlockGroup is a collection of transactionBlocks
type TransactionBlockGroup struct {
	size                    uint8
	lengths                 []uint16
	TransactionBlocks       [][]byte
	TransactionBlockStructs []*TransactionBlock
	merkleTree              *MerkleTree
	finalized               bool
	Height                  uint16
}

//VerifyExceptUTXO checks for every transaction wheter the transaction is ok. It is not checked her whether or not the transaction is already spent with the utxo manager
func (tbg *TransactionBlockGroup) VerifyExceptUTXO() bool {

	if !tbg.TransactionBlockStructs[0].VerifyExceptUTXO(true) {
		return false
	}

	for _, tb := range tbg.TransactionBlockStructs[1:] {
		if !tb.VerifyExceptUTXO(false) {
			return false
		}
	}
	return true

}

// SerializeTransactionBlockGroup makes it ready to send over network
func (tbg *TransactionBlockGroup) SerializeTransactionBlockGroup() []byte {
	totalSize := 3 + int(tbg.size)*2
	for _, v := range tbg.lengths {
		totalSize += int(v)
	}
	ret := make([]byte, totalSize)
	ret[0] = tbg.size
	binary.LittleEndian.PutUint16(ret[1:3], tbg.Height)
	for i, v := range tbg.lengths {
		binary.LittleEndian.PutUint16(ret[3+2*i:5+2*i], v)
	}
	index := 3 + int(tbg.size)*2
	for i, val := range tbg.TransactionBlocks {
		length := int(tbg.lengths[i])
		copy(ret[index:index+length], val[0:length])
		index += length
	}
	return ret
}

// DeserializeTransactionBlockGroup does inverse of SerializeTransactionBlockGroup
func DeserializeTransactionBlockGroup(ret []byte, bc *BlockChain) *TransactionBlockGroup {
	tbg := new(TransactionBlockGroup)
	tbg.size = ret[0]
	tbg.Height = binary.LittleEndian.Uint16(ret[1:3])

	tbg.lengths = make([]uint16, tbg.size)
	tbg.merkleTree = InitializeMerkleTree()

	for i := 0; i < int(tbg.size); i++ {
		tbg.lengths[i] = binary.LittleEndian.Uint16(ret[3+2*i : 5+2*i])
	}

	tbg.TransactionBlocks = make([][]byte, tbg.size)
	tbg.TransactionBlockStructs = make([]*TransactionBlock, tbg.size)

	index := 3 + int(tbg.size)*2
	for i, Uint16Length := range tbg.lengths {
		length := int(Uint16Length)
		tbg.TransactionBlocks[i] = make([]byte, length)
		copy(tbg.TransactionBlocks[i][0:length], ret[index:index+length])
		tbg.TransactionBlockStructs[i] = DeserializeTransactionBlock(tbg.TransactionBlocks[i], bc)
		index += length
		tbg.merkleTree.Add(&tbg.TransactionBlocks[i])
	}
	tbg.merkleTree.FinalizeTree()
	tbg.finalized = true
	return tbg
}

//InitializeTransactionBlockGroup is used to construct tbg
func InitializeTransactionBlockGroup() *TransactionBlockGroup {

	tbg := new(TransactionBlockGroup)
	tbg.size = 0
	tbg.lengths = make([]uint16, 0)
	tbg.TransactionBlocks = make([][]byte, 0)
	tbg.merkleTree = InitializeMerkleTree()
	return tbg
}

//Add adds an element to the group
func (tbg *TransactionBlockGroup) Add(tb *TransactionBlock) {
	if !tbg.finalized {
		stb := tb.SerializeTransactionBlock()
		tbg.size++
		tbg.lengths = append(tbg.lengths, uint16(len(stb)))
		tbg.TransactionBlocks = append(tbg.TransactionBlocks, stb)
		tbg.merkleTree.Add(&stb)
		tbg.TransactionBlockStructs = append(tbg.TransactionBlockStructs, tb)
	} else {
		fmt.Printf("Cannot add to finalized tree, aborting")
	}

}

// FinalizeTransactionBlockGroup needs to be calle before transmission
func (tbg *TransactionBlockGroup) FinalizeTransactionBlockGroup() {
	tbg.merkleTree.FinalizeTree()
	tbg.finalized = true
}
