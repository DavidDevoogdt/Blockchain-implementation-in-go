package main

import "crypto/rsa"

const davidcoin = 1e18

// TransactionSize is len of a transaction
const TransactionSize = 8 + 8 + 2*KeySize + 32

// Wallet keeps track of own money
type Wallet struct {
	PublicKey    [KeySize]byte
	PrivateKey   *rsa.PrivateKey
	Transactions []TransactionRef
	TotalUnspent uint64
	Miner        *Miner
}

// InitializeWallet generates new empty wallet
func InitializeWallet() *Wallet {
	w := new(Wallet)
	priv, pub := GenerateKeyPair()

	w.PrivateKey = priv
	pubArr := PublicKeyToBytes(pub)
	copy(w.PublicKey[0:KeySize], pubArr[0:KeySize])
	//fmt.Printf("%x", w.PublicKey)

	w.TotalUnspent = 0
	w.Transactions = make([]TransactionRef, 0)
	return w
}
