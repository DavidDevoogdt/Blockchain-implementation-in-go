package davidcoin

import (
	"bytes"
	"crypto/rsa"
)

const davidcoin = 1e18

// Wallet keeps track of own money
type Wallet struct {
	PublicKey    [KeySize]byte
	PrivateKey   *rsa.PrivateKey
	Transactions []transactionAndProof
	Miner        *Miner
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
		w.Miner.DebugPrint("head utxo not up to date, could not get amount. Implement this")
		return uint64(0)
	}

	for _, bl := range w.Transactions {
		for _, outp := range bl.tb.OutputList {
			if bytes.Equal(outp.ReceiverPublicKey[0:KeySize], w.PublicKey[0:KeySize]) {
				if utxoptr.VerifyTransactionRef(bl.tr) {
					amount += outp.Amount
				}
			}
		}
	}
	return amount
}

//UpdateWithBlock checks every block whether something was received
func (w *Wallet) UpdateWithBlock(bcn *BlockChainNode) {
	bcn.generalMutex.RLock()
	dp := bcn.DataPointer
	bcn.generalMutex.RUnlock()

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
}
