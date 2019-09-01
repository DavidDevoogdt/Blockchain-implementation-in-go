package davidcoin

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"project/pkg/merkletree"
	rsautil "project/pkg/rsa_util"
)

const outputTextSize = 32

// transactionRefSize is size of a reference tor a transaction in the blockchain
const transactionRefSize = 34

// transactionInputSize is len of a transaction input
const transactionInputSize = rsautil.SignatureSize + transactionRefSize

// transactionOutputSize is len of a transaction output
const transactionOutputSize = 8 + rsautil.SignatureSize + rsautil.KeySize + 32 + outputTextSize

// transactionRef dd
type transactionRef struct {
	BlockHash              [32]byte
	TransactionBlockNumber uint8
	OutputNumber           uint8
}

// SerializeTransactionRef serializes transaction
func (tr *transactionRef) SerializeTransactionRef() [transactionRefSize]byte {
	var ret [transactionRefSize]byte
	copy(ret[0:32], tr.BlockHash[0:32])
	ret[32] = tr.OutputNumber
	ret[33] = tr.TransactionBlockNumber
	return ret
}

// deserializeTransactionRef deserializes transaction
func deserializeTransactionRef(ret [transactionRefSize]byte) *transactionRef {
	tr := new(transactionRef)
	copy(tr.BlockHash[0:32], ret[0:32])
	tr.OutputNumber = ret[32]
	tr.TransactionBlockNumber = ret[33]
	return tr
}

//####################################

// transactionBlockNode keeps usefull information for serialization
type transactionBlockNode struct {
	Hash             [32]byte
	Length           uint64
	TransactionBlock *transactionBlock
}

//serializeTransactionBlockNode makes transaction block ready for transmission
func serializeTransactionBlockNode(tb *transactionBlock, Parent [32]byte) []byte {
	tbn := new(transactionBlockNode)
	ser := tb.serializeTransactionBlock()
	tbn.Length = uint64(len(ser))
	tbn.Hash = tb.hash()
	ret := make([]byte, 32+8+tbn.Length)
	copy(ret[0:32], tbn.Hash[0:32])
	//copy(ret[32:64], tbn.Parent[0:32])
	binary.LittleEndian.PutUint64(ret[32:40], tbn.Length)
	copy(ret[40:40+tbn.Length], ser[:])
	return ret
}

//deserializeTransactionBlockNode makes transaction block ready for transmission. Blockchain is used to recover pointer to actual data, not reference
func deserializeTransactionBlockNode(ret []byte, bc *BlockChain) *transactionBlockNode {
	tbn := new(transactionBlockNode)
	copy(tbn.Hash[0:32], ret[0:32])
	tbn.Length = binary.LittleEndian.Uint64(ret[32:40])
	tbn.TransactionBlock = deserializeTransactionBlock(ret[40:40+tbn.Length], bc)
	return tbn
}

//####################################

// transactionBlock groups transactions
type transactionBlock struct {
	InputNumber                  uint8
	OutputNumber                 uint8
	InputList                    []*transactionInput
	OutputList                   []*transactionOutput
	Signature                    [rsautil.SignatureSize]byte
	PayerPublicKey               [rsautil.KeySize]byte
	UnsolvedReferences           []*unResolvedReferences
	NumberOfUnresolvedReferences uint8
}

type unResolvedReferences struct {
	tr       *transactionRef
	inputnum uint8
	reason   uint8
}

// initializeTransactionBlock setups basic stuff in transactionblock
func initializeTransactionBlock() *transactionBlock {
	tb := new(transactionBlock)
	tb.InputNumber = 0
	tb.OutputNumber = 0
	tb.InputList = make([]*transactionInput, 0)
	tb.OutputList = make([]*transactionOutput, 0)
	return tb
}

// AddOutput ads outp to list
func (tb *transactionBlock) AddOutput(to *transactionOutput) {
	tb.OutputList = append(tb.OutputList, to)
	tb.OutputNumber++
}

// addInput ads outp to list
func (tb *transactionBlock) addInput(ti *transactionInput) {
	tb.InputList = append(tb.InputList, ti)
	tb.InputNumber++
}

// serializeTransactionBlock serializes transactionblock
func (tb *transactionBlock) serializeTransactionBlock() []byte {

	buf := make([]byte, int(tb.InputNumber)*transactionInputSize+int(tb.OutputNumber)*transactionOutputSize+2+rsautil.SignatureSize+rsautil.KeySize)
	buf[0] = tb.InputNumber
	buf[1] = tb.OutputNumber
	ind := 2
	for i := 0; i < int(tb.InputNumber); i++ {
		a := tb.InputList[i].SerializeTransactionInput()
		copy(buf[ind+i*transactionInputSize:ind+(i+1)*transactionInputSize], a[0:transactionInputSize])
	}
	ind = int(tb.InputNumber)*transactionInputSize + 2
	for i := 0; i < int(tb.OutputNumber); i++ {
		a := tb.OutputList[i].serializeTransactionOutput()
		copy(buf[ind+i*transactionOutputSize:ind+(i+1)*transactionOutputSize], a[0:transactionOutputSize])
	}

	ind = int(tb.InputNumber)*transactionInputSize + int(tb.OutputNumber)*transactionOutputSize + 2

	copy(buf[ind:ind+rsautil.KeySize], tb.PayerPublicKey[0:rsautil.KeySize])
	ind += rsautil.KeySize
	copy(buf[ind:ind+rsautil.SignatureSize], tb.Signature[0:rsautil.SignatureSize])

	return buf[:]
}

// deserializeTransactionBlock does the inverse of serialize. The Blockchain is used to recover pointer to dataoutput instead of reference struct
func deserializeTransactionBlock(buf []byte, bc *BlockChain) *transactionBlock {
	tb := new(transactionBlock)
	tb.UnsolvedReferences = make([]*unResolvedReferences, 0)
	inputNumber := uint8(buf[0])
	outputNumber := uint8(buf[1])
	tb.InputNumber = inputNumber
	tb.OutputNumber = outputNumber
	tb.InputList = make([]*transactionInput, inputNumber)
	tb.OutputList = make([]*transactionOutput, outputNumber)
	ind := 2
	for i := 0; i < int(tb.InputNumber); i++ {
		var b [transactionInputSize]byte
		copy(b[0:transactionInputSize], buf[ind+i*transactionInputSize:ind+(i+1)*transactionInputSize])
		inp, ok := deserializeTransactionInput(b, bc)
		tb.InputList[i] = inp
		if ok != uint8(0) {
			tb.NumberOfUnresolvedReferences++
			tb.UnsolvedReferences = append(tb.UnsolvedReferences, &unResolvedReferences{tr: inp.reference, inputnum: uint8(i), reason: ok})
		}
	}
	ind = 2 + int(tb.InputNumber)*transactionInputSize

	for i := 0; i < int(tb.OutputNumber); i++ {
		var b [transactionOutputSize]byte
		copy(b[0:transactionOutputSize], buf[ind+i*transactionOutputSize:ind+(i+1)*transactionOutputSize])

		tb.OutputList[i] = deserializeTransactionOutput(b)
	}
	ind = int(tb.InputNumber)*transactionInputSize + int(tb.OutputNumber)*transactionOutputSize + 2
	copy(tb.PayerPublicKey[0:rsautil.KeySize], buf[ind:ind+rsautil.KeySize])

	ind += rsautil.KeySize
	copy(tb.Signature[0:rsautil.SignatureSize], buf[ind:ind+rsautil.SignatureSize])

	return tb
}

// GetPreSignature fetches the hash of the block without the signature
func (tb *transactionBlock) GetPreSignature() [32]byte {
	ser := tb.serializeTransactionBlock()
	var temp [32]byte
	hash := sha256.Sum256(ser[0 : len(ser)-rsautil.SignatureSize])
	copy(temp[0:32], hash[0:32])
	return temp
}

//signBlock adds the signature to the block given a private key
func (tb *transactionBlock) signBlock(priv *rsa.PrivateKey) {
	pre := tb.GetPreSignature()
	signature := (rsautil.SignWithPrivateKey(pre, priv))
	copy(tb.Signature[0:rsautil.SignatureSize], signature[0:rsautil.SignatureSize])
}

// verifyExceptUTXO is necessary to check whether the coins really belong to the spender, check signatures of block and make sure output is not to large
func (tb *transactionBlock) verifyExceptUTXO(feeBlock bool) bool {
	//fmt.Printf("------------------>todo verify input signatures<------------------\n")

	if !rsautil.VerifyWithPublicKey(tb.GetPreSignature(), tb.Signature, rsautil.BytesToPublicKey(tb.PayerPublicKey)) {
		fmt.Printf("txblock master signature was not good")
		return false
	}
	for _, inp := range tb.InputList {
		if !inp.verify() {
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
		return outputAmount <= inputAmount+1*Davidcoin

	}
	return outputAmount <= inputAmount

}

// hash generates hash of serialized transactionBlock
func (tb *transactionBlock) hash() [32]byte {
	b := tb.serializeTransactionBlock()
	return sha256.Sum256(b[:])

}

//##############################################

// transactionInput is a input transaction (reference to previous output and proof )
type transactionInput struct {
	VerificationChallenge [rsautil.SignatureSize]byte // signed by receiver
	OutputBlock           *transactionOutput          //not serialized
	reference             *transactionRef             // where this block is stored in chain
}

// SerializeTransactionInput puts transaction into byte array
func (ti *transactionInput) SerializeTransactionInput() [transactionInputSize]byte {
	var d [transactionInputSize]byte
	copy(d[0:rsautil.SignatureSize], ti.VerificationChallenge[0:rsautil.SignatureSize])
	temp := ti.reference.SerializeTransactionRef()
	copy(d[rsautil.SignatureSize:rsautil.SignatureSize+transactionRefSize], temp[0:transactionRefSize])
	return d
}

// deserializeTransactionInput turns it back into struct. blockchain is used to recover pointer to data
func deserializeTransactionInput(d [transactionInputSize]byte, bc *BlockChain) (*transactionInput, uint8) {
	ti := new(transactionInput)
	copy(ti.VerificationChallenge[0:rsautil.SignatureSize], d[0:rsautil.SignatureSize])
	var o [transactionRefSize]byte
	copy(o[0:transactionRefSize], d[rsautil.SignatureSize:rsautil.SignatureSize+transactionRefSize])
	ti.reference = deserializeTransactionRef(o)
	ret, ok := bc.getTransactionOutput(ti.reference)
	if ret != nil {
		ti.OutputBlock = ret
	}
	return ti, ok
}

// verify verify ownership of payer
func (ti *transactionInput) verify() bool {
	if ti.OutputBlock == nil {
		fmt.Printf("todo: cannot be verified because outputblock reference is not decoded")
	}
	ser := ti.OutputBlock.serializeTransactionOutput()
	prevHash := sha256.Sum256(ser[:])

	var cont [32]byte
	copy(cont[0:32], prevHash[0:32])

	return rsautil.VerifyWithPublicKey(cont, ti.VerificationChallenge, rsautil.BytesToPublicKey(ti.OutputBlock.ReceiverPublicKey))
}

// sign signs the block
func (ti *transactionInput) sign(priv *rsa.PrivateKey) {
	ser := ti.OutputBlock.serializeTransactionOutput()
	prevHash := sha256.Sum256(ser[:])
	var cont [32]byte
	copy(cont[0:32], prevHash[0:32])

	signa := rsautil.SignWithPrivateKey(cont, priv)
	copy(ti.VerificationChallenge[0:rsautil.SignatureSize], signa[0:rsautil.SignatureSize])
}

//##############################################

// transactionOutput is a Output or output transaction
type transactionOutput struct {
	Amount            uint64
	Signature         [rsautil.SignatureSize]byte //payer signature
	ReceiverPublicKey [rsautil.KeySize]byte
	text              [outputTextSize]byte
}

// serializeTransactionOutput puts transaction into byte array
func (to *transactionOutput) serializeTransactionOutput() [transactionOutputSize]byte {
	var d [transactionOutputSize]byte
	binary.LittleEndian.PutUint64(d[0:8], to.Amount)
	copy(d[8:8+rsautil.KeySize], to.ReceiverPublicKey[0:rsautil.KeySize])
	ind := 8 + rsautil.KeySize
	copy(d[ind:ind+outputTextSize], to.text[0:outputTextSize])
	ind += outputTextSize
	copy(d[ind:ind+rsautil.SignatureSize], to.Signature[0:rsautil.SignatureSize])

	return d
}

// deserializeTransactionOutput turns it back into struct
func deserializeTransactionOutput(d [transactionOutputSize]byte) *transactionOutput {
	to := new(transactionOutput)
	to.Amount = binary.LittleEndian.Uint64(d[0:8])
	copy(to.ReceiverPublicKey[0:rsautil.KeySize], d[8:8+rsautil.KeySize])
	ind := 8 + rsautil.KeySize
	copy(to.text[0:outputTextSize], d[ind:ind+outputTextSize])
	ind += outputTextSize
	copy(to.Signature[0:rsautil.SignatureSize], d[ind:ind+rsautil.SignatureSize])
	return to
}

// presSignatureHash generates hash of serialized transactionBlock
func (to *transactionOutput) presSignatureHash() [32]byte {
	b := to.serializeTransactionOutput()
	return sha256.Sum256(b[0 : transactionOutputSize-rsautil.SignatureSize])
}

// sign is used to proof the payer ordered the spending
func (to *transactionOutput) sign(priv *rsa.PrivateKey) {
	signature := rsautil.SignWithPrivateKey(to.presSignatureHash(), priv)
	copy(to.Signature[0:rsautil.SignatureSize], signature[0:rsautil.SignatureSize])
}

// verify verify ownership of payer
func (to *transactionOutput) verify(PayerPublicKey [rsautil.KeySize]byte) bool {
	return rsautil.VerifyWithPublicKey(to.presSignatureHash(), to.Signature, rsautil.BytesToPublicKey(PayerPublicKey))
}

//###########################################################

// transactionBlockGroup is a collection of transactionBlocks
type transactionBlockGroup struct {
	size                    uint8
	lengths                 []uint16
	transactionBlocks       [][]byte
	TransactionBlockStructs []*transactionBlock
	merkleTree              *merkletree.MerkleTree
	finalized               bool
	Height                  uint16
}

//VerifyExceptUTXO checks for every transaction wheter the transaction is ok. It is not checked her whether or not the transaction is already spent with the utxo manager
func (tbg *transactionBlockGroup) VerifyExceptUTXO() bool {

	if !tbg.TransactionBlockStructs[0].verifyExceptUTXO(true) {
		return false
	}

	for _, tb := range tbg.TransactionBlockStructs[1:] {
		if !tb.verifyExceptUTXO(false) {
			return false
		}
	}
	return true

}

// serializeTransactionBlockGroup makes it ready to send over network
func (tbg *transactionBlockGroup) serializeTransactionBlockGroup() []byte {
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
	for i, val := range tbg.transactionBlocks {
		length := int(tbg.lengths[i])
		copy(ret[index:index+length], val[0:length])
		index += length
	}
	return ret
}

// deserializeTransactionBlockGroup does inverse of SerializeTransactionBlockGroup
func deserializeTransactionBlockGroup(ret []byte, bc *BlockChain) *transactionBlockGroup {
	tbg := new(transactionBlockGroup)
	tbg.size = ret[0]
	tbg.Height = binary.LittleEndian.Uint16(ret[1:3])

	tbg.lengths = make([]uint16, tbg.size)
	tbg.merkleTree = merkletree.InitializeMerkleTree()

	for i := 0; i < int(tbg.size); i++ {
		tbg.lengths[i] = binary.LittleEndian.Uint16(ret[3+2*i : 5+2*i])
	}

	tbg.transactionBlocks = make([][]byte, tbg.size)
	tbg.TransactionBlockStructs = make([]*transactionBlock, tbg.size)

	index := 3 + int(tbg.size)*2
	for i, Uint16Length := range tbg.lengths {
		length := int(Uint16Length)
		tbg.transactionBlocks[i] = make([]byte, length)
		copy(tbg.transactionBlocks[i][0:length], ret[index:index+length])
		tbg.TransactionBlockStructs[i] = deserializeTransactionBlock(tbg.transactionBlocks[i], bc)
		index += length
		tbg.merkleTree.Add(&tbg.transactionBlocks[i])
	}
	tbg.merkleTree.FinalizeTree()
	tbg.finalized = true
	return tbg
}

//initializeTransactionBlockGroup is used to construct tbg
func initializeTransactionBlockGroup() *transactionBlockGroup {

	tbg := new(transactionBlockGroup)
	tbg.size = 0
	tbg.lengths = make([]uint16, 0)
	tbg.transactionBlocks = make([][]byte, 0)
	tbg.merkleTree = merkletree.InitializeMerkleTree()
	return tbg
}

//Add adds an element to the group
func (tbg *transactionBlockGroup) Add(tb *transactionBlock) {
	if !tbg.finalized {
		stb := tb.serializeTransactionBlock()
		tbg.size++
		tbg.lengths = append(tbg.lengths, uint16(len(stb)))
		tbg.transactionBlocks = append(tbg.transactionBlocks, stb)
		tbg.merkleTree.Add(&stb)
		tbg.TransactionBlockStructs = append(tbg.TransactionBlockStructs, tb)
	} else {
		fmt.Printf("Cannot add to finalized tree, aborting")
	}

}

// finalizeTransactionBlockGroup needs to be calle before transmission
func (tbg *transactionBlockGroup) finalizeTransactionBlockGroup() {
	tbg.merkleTree.FinalizeTree()
	tbg.finalized = true
}
