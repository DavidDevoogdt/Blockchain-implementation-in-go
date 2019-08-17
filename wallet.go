package main

// TransactionSize is len of a transaction
const TransactionSize = 8 + 8 + 2*KeySize + 32

// Wallet keeps track of own money
type Wallet struct {
	PublicKey    [KeySize]byte
	PrivateKey   [KeySize]byte
	Transactions []TransactionRef
	TotalUnspent uint64
	Miner        *Miner
}

// InitializeWallet generates new empty wallet
func InitializeWallet() *Wallet {
	w := new(Wallet)
	priv, pub := GenerateKeyPair()

	w.PrivateKey = PrivateKeyToBytes(priv)
	w.PublicKey = PublicKeyToBytes(pub)
	//fmt.Printf("%x", w.PublicKey)

	w.TotalUnspent = 0
	w.Transactions = make([]TransactionRef, 0)
	return w
}
