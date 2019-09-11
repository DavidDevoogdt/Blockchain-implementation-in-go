package davidcoin

import (
	"bytes"
	"crypto/rsa"
	"fmt"
	"log"
	rsautil "project/pkg/rsa_util"

	"github.com/sasha-s/go-deadlock"
)

// Davidcoin is the smallelt currency: 1e9 davidacoin = 1 coin
const Davidcoin = 1e9

// Wallet keeps track of own money
type Wallet struct {
	PublicKey    [rsautil.KeySize]byte
	privateKey   *rsa.PrivateKey
	transactions []transactionAndProof
	Miner        *Miner

	deadlock.RWMutex
}

type transactionAndProof struct {
	tb *transactionBlock
	tp []byte //transactionproof stores the merkle tree proof and the transactionblock
	tr *transactionRef
}

// initializeWallet generates new empty wallet
func (m *Miner) initializeWallet() *Wallet {
	w := new(Wallet)
	w.Miner = m
	priv, pub := rsautil.GenerateKeyPair()

	w.privateKey = priv
	pubArr := rsautil.PublicKeyToBytes(pub)
	copy(w.PublicKey[0:rsautil.KeySize], pubArr[0:rsautil.KeySize])
	//fmt.Printf("%x", w.PublicKey)

	w.transactions = make([]transactionAndProof, 0)
	return w
}

// TotalAvailable checks all transactionrefs against the utxo manager of the head. If unspent argument is true, the money for transaction being vallidated is not count
func (w *Wallet) TotalAvailable() uint64 {
	amount := uint64(0)

	bc := w.Miner.BlockChain
	bc.blockChainMutex.RLock()
	head := bc.head
	bc.blockChainMutex.RUnlock()

	head.generalMutex.RLock()
	uptodate := head.uTxOManagerIsUpToDate
	utxoptr := head.uTxOManagerPointer
	head.generalMutex.RUnlock()

	if !uptodate {
		w.Miner.debugPrint("head utxo not up to date, could not get amount. Implement this\n")
		return uint64(0)
	}

	w.RLock()
	for _, bl := range w.transactions {
		outp := bl.tb.OutputList[bl.tr.OutputNumber]

		check := bytes.Equal(outp.ReceiverPublicKey[0:rsautil.KeySize], w.PublicKey[0:rsautil.KeySize])
		if !check {
			log.Fatal("wallet contains transaction output for other miner\n")
		}

		goodTrans := utxoptr.verifyTransactionRef(bl.tr)
		if goodTrans {

			amount += outp.Amount

		}
	}
	w.RUnlock()

	return amount
}

//MakeTransaction gather the resources, makes transactionblock and broadcasts it
func (w *Wallet) MakeTransaction(Receiver [rsautil.KeySize]byte, msg string, amount uint64) bool {

	currentAmount := uint64(0)

	tx := initializeTransactionBlock()
	tx.PayerPublicKey = w.PublicKey

	bc := w.Miner.BlockChain
	bc.blockChainMutex.RLock()
	head := bc.head
	bc.blockChainMutex.RUnlock()

	head.generalMutex.RLock()
	uptodate := head.uTxOManagerIsUpToDate
	utxoptr := head.uTxOManagerPointer
	head.generalMutex.RUnlock()

	if !uptodate {
		//w.Miner.DebugPrint("head utxo not up to date, verify transaction\n")
		fmt.Printf("head utxo is not up to date, stopping transaction\n")
		return false
	}
	w.RLock()
	for _, bl := range w.transactions {
		to := bl.tb.OutputList[bl.tr.OutputNumber]

		goodtrans := utxoptr.verifyTransactionRef(bl.tr)
		if goodtrans {
			ti := new(transactionInput)
			ti.reference = bl.tr
			ti.OutputBlock = to
			ti.sign(w.privateKey)
			tx.addInput(ti)
			currentAmount += to.Amount

			if currentAmount >= amount {
				break
			}

		}

	}
	w.RUnlock()

	if currentAmount < amount {
		w.Miner.debugPrint("Not Enough money to make payment, aborting\n")
		//fmt.Printf("Not Enough money to make payment, aborting\n")
		return false
	}

	exchange := currentAmount - amount

	to := new(transactionOutput)
	to.Amount = amount
	to.ReceiverPublicKey = Receiver
	copy(to.text[0:32], msg)
	to.sign(w.privateKey)

	tx.AddOutput(to)

	if exchange != uint64(0) {
		to2 := new(transactionOutput)
		to2.Amount = exchange
		copy(to2.text[0:32], "exchange")
		to2.ReceiverPublicKey = w.PublicKey
		to2.sign(w.privateKey)
		tx.AddOutput(to2)
	}

	tx.signBlock(w.privateKey)

	//bool1 := tx.VerifyExceptUTXO(false)
	//bool2 := utxoptr.VerifyTransactionBlockRefs(tx)

	ser := tx.serializeTransactionBlock()
	go w.Miner.send(sendType["Transaction"], ser[:], w.PublicKey, Everyone)

	if w.Miner.tp != nil {
		w.Miner.tp.receiveInternal(tx)
	}
	//w.Miner.DebugPrint("Made transaction\n")
	w.Miner.debugPrint(fmt.Sprintf("Made transaction amount = %.4f \n", float64(amount)/Davidcoin))

	return true
}

//updateWithBlock checks every block whether something was received
func (w *Wallet) updateWithBlock(bcn *BlockChainNode) {
	bcn.generalMutex.RLock()
	dp := bcn.dataPointer
	bcn.generalMutex.RUnlock()

	w.Lock()

	for i, p := range dp.TransactionBlockStructs {
		for j, tro := range p.OutputList {
			if bytes.Equal(tro.ReceiverPublicKey[0:rsautil.KeySize], w.PublicKey[0:rsautil.KeySize]) {
				proof := dp.merkleTree.GenerareteMerkleProof(uint8(i))
				serProof := proof.SerializeProofStruct()
				w.transactions = append(w.transactions, transactionAndProof{
					tb: p,
					tp: serProof,
					tr: &transactionRef{
						OutputNumber:           uint8(j),
						BlockHash:              bcn.Hash,
						TransactionBlockNumber: uint8(i),
					},
				})
			}
		}
	}

	w.Unlock()
}

//Print prints current cash status
func (w *Wallet) Print() {
	fmt.Printf("%s has %.3f davidcoin\n", w.Miner.Name, float64(w.TotalAvailable())/Davidcoin)
}
