package davidcoin

import (
	"bytes"
	"crypto/rsa"
	"fmt"

	"github.com/sasha-s/go-deadlock"
)

const davidcoin = 1e18

// Wallet keeps track of own money
type Wallet struct {
	PublicKey    [KeySize]byte
	PrivateKey   *rsa.PrivateKey
	Transactions []transactionAndProof
	Miner        *Miner

	deadlock.RWMutex
}

type transactionAndProof struct {
	tb *TransactionBlock
	tp []byte //transactionproof stores the merkle tree proof and the transactionblock
	tr *TransactionRef
}

// InitializeWallet generates new empty wallet
func (m *Miner) InitializeWallet() *Wallet {
	w := new(Wallet)
	w.Miner = m
	priv, pub := GenerateKeyPair()

	w.PrivateKey = priv
	pubArr := PublicKeyToBytes(pub)
	copy(w.PublicKey[0:KeySize], pubArr[0:KeySize])
	//fmt.Printf("%x", w.PublicKey)

	w.Transactions = make([]transactionAndProof, 0)
	return w
}

// TotalUnspent checks all transactionrefs against the utxo manager of the head
func (w *Wallet) TotalUnspent() uint64 {
	amount := uint64(0)

	bc := w.Miner.BlockChain
	bc.BlockChainMutex.RLock()
	head := bc.Head
	bc.BlockChainMutex.RUnlock()

	head.generalMutex.RLock()
	uptodate := head.UTxOManagerIsUpToDate
	utxoptr := head.UTxOManagerPointer
	head.generalMutex.RUnlock()

	if !uptodate {
		w.Miner.DebugPrint("head utxo not up to date, could not get amount. Implement this\n")
		return uint64(0)
	}

	w.RLock()

	for _, bl := range w.Transactions {
		for _, outp := range bl.tb.OutputList {
			if bytes.Equal(outp.ReceiverPublicKey[0:KeySize], w.PublicKey[0:KeySize]) {
				if utxoptr.VerifyTransactionRef(bl.tr) {
					amount += outp.Amount
				}
			}
		}
	}
	w.RUnlock()

	return amount
}

//UpdateWithBlock checks every block whether something was received
func (w *Wallet) UpdateWithBlock(bcn *BlockChainNode) {
	bcn.generalMutex.RLock()
	dp := bcn.DataPointer
	bcn.generalMutex.RUnlock()

	w.Lock()

	for i, p := range dp.TransactionBlockStructs {
		for j, tro := range p.OutputList {
			if bytes.Equal(tro.ReceiverPublicKey[0:KeySize], w.PublicKey[0:KeySize]) {
				proof := dp.merkleTree.GenerareteMerkleProof(uint8(i))
				serProof := proof.SerializeProofStruct()
				w.Transactions = append(w.Transactions, transactionAndProof{
					tb: p,
					tp: serProof,
					tr: &TransactionRef{
						OutputNumber:           uint8(j),
						BlockHash:              bcn.Hash,
						TransactionBlockNumber: uint8(i),
					},
				})
				break
			}
		}
	}

	w.Unlock()
}

//Print prints current cash status
func (w *Wallet) Print() {
	fmt.Printf("%s has %.3f davidcoin\n", w.Miner.Name, float64(w.TotalUnspent())/davidcoin)
}

//MakeTransaction gather the resources, makes transactionblock and broadcasts it
func (w *Wallet) MakeTransaction(Receiver [KeySize]byte, msg string, amount uint64) bool {

	currentAmount := uint64(0)

	tx := InitializeTransactionBlock()
	tx.PayerPublicKey = w.PublicKey

	bc := w.Miner.BlockChain
	bc.BlockChainMutex.RLock()
	head := bc.Head
	bc.BlockChainMutex.RUnlock()

	head.generalMutex.RLock()
	uptodate := head.UTxOManagerIsUpToDate
	utxoptr := head.UTxOManagerPointer
	head.generalMutex.RUnlock()

	if !uptodate {
		w.Miner.DebugPrint("head utxo not up to date, verify transaction\n")
		return false
	}
	w.Lock()
	for _, a := range w.Transactions {
		to := a.tb.OutputList[a.tr.OutputNumber]

		if utxoptr.VerifyTransactionRef(a.tr) {
			ti := new(TransactionInput)
			ti.reference = a.tr
			ti.OutputBlock = to
			ti.Sign(w.PrivateKey)
			tx.AddInput(ti)
			currentAmount += to.Amount

			if currentAmount >= to.Amount {
				break
			}

		}
	}
	w.Unlock()

	if currentAmount < amount {
		w.Miner.DebugPrint("Not Enough money to make payment, aborting\n")
		return false
	}

	exchange := currentAmount - amount

	to := new(TransactionOutput)
	to.Amount = amount
	to.ReceiverPublicKey = Receiver
	copy(to.text[0:32], msg)
	to.Sign(w.PrivateKey)

	tx.AddOutput(to)

	if exchange != uint64(0) {
		to2 := new(TransactionOutput)
		to2.Amount = exchange
		copy(to2.text[0:32], "exchange")
		to2.ReceiverPublicKey = w.PublicKey
		to2.Sign(w.PrivateKey)
		tx.AddOutput(to2)
	}

	tx.SignBlock(w.PrivateKey)

	//bool1 := tx.VerifyExceptUTXO(false)
	//bool2 := utxoptr.VerifyTransactionBlockRefs(tx)

	ser := tx.SerializeTransactionBlock()
	go w.Miner.Send(SendType["Transaction"], ser[:], w.PublicKey, Everyone)

	if w.Miner.tp != nil {
		w.Miner.tp.ReceiveInternal(tx)
	}
	w.Miner.DebugPrint("Made transaction\n")

	return true
}
