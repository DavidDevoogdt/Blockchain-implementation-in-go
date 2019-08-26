package main

import (
	"fmt"
	"sync"
)

// UTxOManager keeps track of unspent transaction outputs (utxo)
type UTxOManager struct {
	attachedBlockChainNode *BlockChainNode
	UtxOMap                map[[32]byte]map[uint8]map[uint8]bool //merkleroot -> tx number -> output number
	mapMutex               sync.RWMutex
	secondLayerMutexes     [256]sync.RWMutex
	thirdLayerMutexes      [256]sync.RWMutex //to protect all the nested maps, a common set of mutexes is used irrespective of the block hash. This is lightweight and still faster than using a single mutex for all
	Miner                  *Miner
}

// InitializeUTxOMananger is used to initialize the blockchain
func InitializeUTxOMananger(m *Miner) *UTxOManager {
	utxo := new(UTxOManager)
	utxo.UtxOMap = make(map[[32]byte]map[uint8]map[uint8]bool)
	utxo.Miner = m
	utxo.attachedBlockChainNode = m.BlockChain.Root
	utxo.attachedBlockChainNode.HasData = false
	utxo.attachedBlockChainNode.UTxOManagerIsUpToDate = true
	return utxo
}

// VerifyTransactionRef checks whether a transactionref is spendable
func (um *UTxOManager) VerifyTransactionRef(tr *TransactionRef) bool {
	um.mapMutex.RLock()
	a, ok := um.UtxOMap[tr.BlockHash]
	um.mapMutex.RUnlock()

	if !ok {
		fmt.Printf("Transaction merkle root not in chain, invalid transaction")
		return false
	}

	um.secondLayerMutexes[tr.OutputNumber].RLock()
	b, ok2 := a[tr.TransactionBlockNumber]
	um.secondLayerMutexes[tr.OutputNumber].RUnlock()

	if !ok2 {
		fmt.Printf("Transaction block number not in chain, invalid transaction")
		return false
	}

	um.thirdLayerMutexes[tr.OutputNumber].RLock()
	c, ok3 := b[tr.OutputNumber]
	um.thirdLayerMutexes[tr.OutputNumber].RUnlock()

	if !ok3 {
		fmt.Printf("Transaction already spent or does not exist")
		return false
	}
	return c // still could be false
}

// VerifyTransactionBlockRefs only checks whether or not the transactions in the inputlist are spendable, not the signature and such
func (um *UTxOManager) VerifyTransactionBlockRefs(tbg *TransactionBlockGroup) bool {

	for _, tb := range tbg.TransactionBlockStructs {
		for _, b := range tb.InputList[:] {

			if !um.VerifyTransactionRef(b.reference) {
				return false
			}
		}
	}
	return true
}

// UpdateWithNextBlockChainNode adds new data to map. Deepcopy preserves map to previous block
func (um *UTxOManager) UpdateWithNextBlockChainNode(bcn *BlockChainNode, deepCopy bool) (succeeded bool) {
	succeeded = true

	var wgDeepCopy sync.WaitGroup

	var umn *UTxOManager
	if deepCopy {
		wgDeepCopy.Add(1)
		go func() {
			umn = um.Deepcopy()
			wgDeepCopy.Done()
		}()

	}
	_ = umn

	tbg := bcn.DataPointer

	if tbg == nil {
		fmt.Printf("No data attached to block, not updating")
		succeeded = false
		return
	}
	if bcn.PrevBlockChainNode == nil {
		fmt.Printf("bcn has no pref blockchainnode, returning")
		return false
	}

	//if um == nil {
	//	fmt.Printf("um is nil, returning\n")
	//	return false
	//}

	if bcn.PrevBlockChainNode != um.attachedBlockChainNode {
		fmt.Printf("updating with the wrong blockchainnode, not updating")
		return false
	}
	if !bcn.PrevBlockChainNode.UTxOManagerIsUpToDate {
		fmt.Printf("Previous utxo manager was not up to date, stopping ")
	}

	var waitGroup sync.WaitGroup

	// update every transactionBlock Async
	for _, tb := range tbg.TransactionBlockStructs {
		waitGroup.Add(int(tb.InputNumber) + int(tb.OutputNumber))
		go func(tb *TransactionBlock) {
			for i := range tb.InputList {
				txInput := tb.InputList[i]
				go func() {
					tr := txInput.reference
					um.mapMutex.RLock()
					a, ok := um.UtxOMap[tr.BlockHash]
					um.mapMutex.RUnlock()

					if !ok {
						fmt.Printf("UTxO was not in sync, merkle missing!, utxo corrupted")
						succeeded = false

					}

					um.secondLayerMutexes[tr.OutputNumber].RLock()
					b, ok2 := a[tr.TransactionBlockNumber]
					um.secondLayerMutexes[tr.OutputNumber].RUnlock()

					if !ok2 {
						fmt.Printf("UTxO was not in sync, transaction block missing!, utxo corrupted")
						succeeded = false
					}

					um.thirdLayerMutexes[tr.OutputNumber].RLock()
					_, ok3 := b[tr.OutputNumber]
					um.thirdLayerMutexes[tr.OutputNumber].RUnlock()

					if !ok3 {
						fmt.Printf("UTxO was not in sync, transaction output missing!, utxo corrupted")
						succeeded = false
					}

					um.thirdLayerMutexes[tr.OutputNumber].Lock()
					delete(b, tr.OutputNumber)
					um.thirdLayerMutexes[tr.OutputNumber].Lock()

					waitGroup.Done()
				}()
			}

			um.mapMutex.RLock()
			a, ok := um.UtxOMap[bcn.Hash]
			um.mapMutex.RUnlock()

			if !ok {
				a = make(map[uint8]map[uint8]bool)
				um.mapMutex.Lock()
				um.UtxOMap[bcn.Hash] = a
				um.mapMutex.Unlock()
			}

			for i, x := range tbg.TransactionBlockStructs {
				ui := uint8(i)

				um.secondLayerMutexes[i].RLock()
				b, ok2 := a[ui]
				um.secondLayerMutexes[i].RUnlock()

				if !ok2 {
					b = make(map[uint8]bool)
					um.secondLayerMutexes[i].Lock()
					a[ui] = b
					um.secondLayerMutexes[i].Unlock()
				}

				for j := range x.OutputList {
					go func(j uint8) {
						um.thirdLayerMutexes[j].Lock()
						b[j] = true
						um.thirdLayerMutexes[j].Unlock()

						waitGroup.Done()
					}(uint8(j))
				}

			}

		}(tb)
	}

	waitGroup.Wait()

	if !succeeded {
		fmt.Printf("Some record could not be correctly updated in the map, probably missing")
		return
	}

	um.attachedBlockChainNode = bcn

	if deepCopy {
		bcn.PrevBlockChainNode.UTxOManagerIsUpToDate = true
		wgDeepCopy.Wait() //was done async, needs to be done now
		bcn.PrevBlockChainNode.UTxOManagerPointer = umn
		umn.attachedBlockChainNode = bcn.PrevBlockChainNode
	} else {
		bcn.PrevBlockChainNode.UTxOManagerIsUpToDate = false
		bcn.PrevBlockChainNode.UTxOManagerPointer = nil
	}

	um.attachedBlockChainNode.UTxOManagerIsUpToDate = true
	um.attachedBlockChainNode.UTxOManagerPointer = um

	return true
}

// Deepcopy generates exact copy at different memory location
func (um *UTxOManager) Deepcopy() *UTxOManager {
	var wg sync.WaitGroup
	umCopy := new(UTxOManager)
	umCopy.attachedBlockChainNode = um.attachedBlockChainNode
	umCopy.UtxOMap = make(map[[32]byte]map[uint8]map[uint8]bool)

	// acquire all locks for second layer
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RLock()
	}

	// acquire all locks for third layer
	for i := range um.thirdLayerMutexes {
		um.thirdLayerMutexes[i].RLock()
	}

	um.mapMutex.RLock()
	for k, v := range um.UtxOMap {
		wg.Add(1)

		go func(v map[uint8]map[uint8]bool, k [32]byte) {
			newMap := make(map[uint8]map[uint8]bool)
			for k2, v2 := range v {
				wg.Add(1)

				go func(v2 map[uint8]bool, k2 uint8) {
					newMap2 := make(map[uint8]bool)
					for k3, v3 := range v2 {
						newMap2[k3] = v3
					}
					umCopy.secondLayerMutexes[k2].Lock()
					newMap[k2] = newMap2
					umCopy.secondLayerMutexes[k2].Unlock()

					wg.Done()
				}(v2, k2)
			}
			umCopy.mapMutex.Lock()
			umCopy.UtxOMap[k] = newMap
			umCopy.mapMutex.Unlock()

			wg.Done()
		}(v, k)

	}
	um.mapMutex.RUnlock()

	wg.Wait()

	// release all locks for second layer
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RUnlock()
	}

	// release all locks for third layer
	for i := range um.thirdLayerMutexes {
		um.thirdLayerMutexes[i].RUnlock()
	}

	return umCopy
}

//Print shows all unspent transactions
func (um *UTxOManager) Print() {

	var wg sync.WaitGroup

	// acquire all locks for second layer
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RLock()
	}

	// acquire all locks for third layer
	for i := range um.thirdLayerMutexes {
		um.thirdLayerMutexes[i].RLock()
	}

	um.mapMutex.RLock()
	for k, v := range um.UtxOMap {
		wg.Add(1)

		go func(v map[uint8]map[uint8]bool, k [32]byte) {

			for k2, v2 := range v {
				wg.Add(1)
				go func(v2 map[uint8]bool, k2 uint8) {

					for k3 := range v2 {
						fmt.Printf("transaction %x-%d-%d unspent\n", k, k2, k3)
					}

					wg.Done()
				}(v2, k2)
			}
			wg.Done()
		}(v, k)

	}
	um.mapMutex.RUnlock()

	wg.Wait()

	// release all locks for second layer
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RUnlock()
	}

	// release all locks for third layer
	for i := range um.thirdLayerMutexes {
		um.thirdLayerMutexes[i].RUnlock()
	}

}
