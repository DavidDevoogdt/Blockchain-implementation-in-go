package davidcoin

import (
	"crypto/sha256"
	"encoding/binary"
)

// TransactionRefSize is size of a reference tor a transaction in the blockchain
const TransactionRefSize = 33

// TransactionInputSize is len of a transaction input
const TransactionInputSize = 32 + TransactionRefSize

// TransactionOutputSize is len of a transaction output
const TransactionOutputSize = 8 + 32 + 2*KeySize + TransactionRefSize

// TransactionRequestSize is len of a transaction request
const TransactionRequestSize = 8 + 32 + 2*KeySize

// TransactionRef dd
type TransactionRef struct {
	BlockHash [32]byte
	Number    uint8
}

// SerializeTransactionRef serializes transaction
func (tr *TransactionRef) SerializeTransactionRef() [TransactionRefSize]byte {
	var ret [TransactionRefSize]byte
	copy(ret[0:32], tr.BlockHash[0:32])
	ret[32] = tr.Number
	return ret
}

// DeserializeTransactionRef deserializes transaction
func DeserializeTransactionRef(ret [TransactionRefSize]byte) *TransactionRef {
	tr := new(TransactionRef)
	copy(tr.BlockHash[0:32], ret[0:32])
	tr.Number = ret[32]
	return tr
}

//####################################

// TransactionBlockNode keeps usefull information for serialization
type TransactionBlockNode struct {
	Hash             [32]byte
	Parent           [32]byte
	Length           uint64
	TransactionBlock *TransactionBlock
}

//SerializeTransactionBlockNode makes transaction block ready for transmission
func SerializeTransactionBlockNode(tb *TransactionBlock, Parent [32]byte) []byte {
	tbn := new(TransactionBlockNode)
	ser := tb.SerializeTransactionBlock()
	tbn.Length = uint64(len(ser))
	tbn.Hash = tb.Hash()
	ret := make([]byte, 32+32+8+tbn.Length)
	copy(ret[0:32], tbn.Hash[0:32])
	copy(ret[32:64], tbn.Parent[0:32])
	binary.LittleEndian.PutUint64(ret[64:72], tbn.Length)
	copy(ret[72:72+tbn.Length], ser[:])
	return ret
}

//DeserializeTransactionBlockNode makes transaction block ready for transmission
func DeserializeTransactionBlockNode(ret []byte) *TransactionBlockNode {
	tbn := new(TransactionBlockNode)

	copy(tbn.Hash[0:32], ret[0:32])
	copy(tbn.Parent[0:32], ret[32:64])
	tbn.Length = binary.LittleEndian.Uint64(ret[64:72])
	tbn.TransactionBlock = DeserializeTransactionBlock(ret[72 : 72+tbn.Length])
	return tbn
}

//####################################

// TransactionBlock groups transactions
type TransactionBlock struct {
	InputNumber  uint8
	OutputNumber uint8
	InputList    []*TransactionInput
	OutputList   []*TransactionOutput
	//InputList    [][TransactionInputSize]byte
	//OutputList   [][TransactionOutputSize]byte
}

// SerializeTransactionBlock serializes transactionblock
func (tb *TransactionBlock) SerializeTransactionBlock() []byte {

	buf := make([]byte, int(tb.InputNumber)*TransactionSize+int(tb.OutputNumber)*TransactionSize+2*1)
	buf[0] = tb.InputNumber
	buf[1] = tb.OutputNumber
	for i := 0; i < int(tb.InputNumber); i++ {
		a := tb.InputList[i].SerializeTransactionInput()
		copy(buf[2+i*TransactionSize:2+(i+1)*TransactionSize], a[0:TransactionInputSize])
	}
	for i := 0; i < int(tb.OutputNumber); i++ {
		a := tb.OutputList[i].SerializeTransactionOutput()
		copy(buf[2+int(tb.InputNumber)*TransactionInputSize+i*TransactionOutputSize:2+int(tb.InputNumber)*TransactionInputSize+(i+1)*TransactionOutputSize], a[0:TransactionOutputSize])
	}
	return buf[:]
}

// DeserializeTransactionBlock does the inverse of serialize
func DeserializeTransactionBlock(buf []byte) *TransactionBlock {
	tb := new(TransactionBlock)
	inputNumber := uint8(buf[0])
	outputNumber := uint8(buf[1])
	tb.InputNumber = inputNumber
	tb.OutputNumber = outputNumber
	tb.InputList = make([]*TransactionInput, inputNumber)
	tb.OutputList = make([]*TransactionOutput, outputNumber)
	for i := 0; i < int(tb.InputNumber); i++ {
		var b [TransactionInputSize]byte
		copy(b[0:TransactionInputSize], buf[2+i*TransactionInputSize:2+(i+1)*TransactionInputSize])
		tb.InputList[i] = DeserializeTransactionInput(b)
	}
	for i := 0; i < int(tb.OutputNumber); i++ {
		var b [TransactionOutputSize]byte
		copy(b[0:TransactionOutputSize], buf[2+int(tb.InputNumber)*TransactionInputSize+i*TransactionOutputSize:2+int(tb.InputNumber)*TransactionInputSize+(i+1)*TransactionOutputSize])

		tb.OutputList[i] = DeserializeTransactionOutput(b)
	}
	return tb
}

// Hash generates hash of serialized transactionBlock
func (tb *TransactionBlock) Hash() [32]byte {
	b := tb.SerializeTransactionBlock()
	return sha256.Sum256(b[:])

}

//##############################################

// TransactionInput is a input transaction (reference to previous output and proof )
type TransactionInput struct {
	VerificationChallenge [32]byte // signed by receiver
	OutputBlock           *TransactionOutput
}

// SerializeTransactionInput puts transaction into byte array
func (tx *TransactionInput) SerializeTransactionInput() [TransactionInputSize]byte {
	var d [TransactionInputSize]byte
	copy(d[0:32], tx.VerificationChallenge[0:32])
	temp := tx.OutputBlock.reference.SerializeTransactionRef()
	copy(d[32:32+TransactionRefSize], temp[0:TransactionRefSize])
	return d
}

// DeserializeTransactionInput turns it back into struct
func DeserializeTransactionInput(d [TransactionInputSize]byte) *TransactionInput {
	tx := new(TransactionInput)
	copy(tx.VerificationChallenge[0:32], d[0:32])
	var o [TransactionRefSize]byte
	copy(o[0:TransactionRefSize], d[32:32+TransactionRefSize])
	tx.OutputBlock.reference = DeserializeTransactionRef(o)
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
	Signature         [32]byte //payer signature
	PayerPublicKey    [KeySize]byte
	ReceiverPublicKey [KeySize]byte
	reference         *TransactionRef // where this block is stored in chain
}

// SerializeTransactionOutput puts transaction into byte array
func (tx *TransactionOutput) SerializeTransactionOutput() [TransactionOutputSize]byte {
	var d [TransactionOutputSize]byte
	binary.LittleEndian.PutUint64(d[0:8], tx.Amount)
	copy(d[8:8+KeySize], tx.PayerPublicKey[0:KeySize])
	copy(d[8+KeySize:8+2*KeySize], tx.ReceiverPublicKey[0:KeySize])
	copy(d[8+2*KeySize:8+2*KeySize+32], tx.Signature[0:32])
	temp := tx.reference.SerializeTransactionRef()
	copy(d[8+2*KeySize+32:8+2*KeySize+32+TransactionRefSize], temp[0:TransactionRefSize])
	return d
}

// DeserializeTransactionOutput turns it back into struct
func DeserializeTransactionOutput(d [TransactionOutputSize]byte) *TransactionOutput {
	tx := new(TransactionOutput)
	tx.Amount = binary.LittleEndian.Uint64(d[0:8])
	copy(tx.PayerPublicKey[0:KeySize], d[8:8+KeySize])
	copy(tx.ReceiverPublicKey[0:KeySize], d[8+KeySize:8+2*KeySize])
	copy(tx.Signature[0:32], d[8+2*KeySize:8+2*KeySize+32])
	var temp [TransactionRefSize]byte
	copy(temp[0:TransactionRefSize], d[8+2*KeySize+32:8+2*KeySize+32+TransactionRefSize])
	tx.reference = DeserializeTransactionRef(temp)
	return tx
}

// SerializeTransactionRequest puts transaction into byte array
func (tx *TransactionOutput) SerializeTransactionRequest() [TransactionRequestSize]byte {
	var d [TransactionRequestSize]byte
	binary.LittleEndian.PutUint64(d[0:8], tx.Amount)
	copy(d[8:8+KeySize], tx.PayerPublicKey[0:KeySize])
	copy(d[8+KeySize:8+2*KeySize], tx.ReceiverPublicKey[0:KeySize])
	copy(d[8+2*KeySize:8+2*KeySize+32], tx.Signature[0:32])
	return d
}

// Hash generates hash of serialized transactionBlock
func (tx *TransactionOutput) Hash() [32]byte {
	b := tx.SerializeTransactionOutput()
	return sha256.Sum256(b[0 : TransactionOutputSize-32])
}

// Verify verify ownership of payer
func (tx *TransactionOutput) Verify() bool {
	return VerifyWithPublicKey(tx.Hash(), tx.Signature, BytesToPublicKey(tx.PayerPublicKey))
}
