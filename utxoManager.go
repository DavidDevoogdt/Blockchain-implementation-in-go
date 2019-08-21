package main

import (
	"fmt"
	"sync"
)

// UTxOManager keeps track of unspent transaction outputs (utxo)
type UTxOManager struct {
	attachedBlockChainNode *BlockChainNode
	UtxOMap                map[[32]byte]map[uint8]bool
	mapMutex               sync.RWMutex
	secondLayerMutexes     [256]sync.RWMutex //to protect all the nested maps, a common set of mutexes is used irrespective of the block number. This is lightweight and still faster than using a single mutex for all
	Miner                  *Miner
}

// InitializeUTxOMananger is used to initialize the blockchain
func InitializeUTxOMananger(m *Miner) *UTxOManager {
	utxo := new(UTxOManager)
	utxo.UtxOMap = make(map[[32]byte]map[uint8]bool)
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
		fmt.Printf("Transaction not in chain, invalid transaction")
		return false
	}

	um.secondLayerMutexes[tr.Number].RLock()
	b, ok2 := a[tr.Number]
	um.secondLayerMutexes[tr.Number].RUnlock()

	if !ok2 {
		fmt.Printf("Transaction already spent or does not exist")
		return false
	}
	return b // still could be false
}

// VerifyTransactionBlockRefs only checks whether or not the transactions in the inputlist are spendable, not the signature and such
func (um *UTxOManager) VerifyTransactionBlockRefs(tb *TransactionBlock) bool {
	for _, b := range tb.InputList[:] {

		if !um.VerifyTransactionRef(b.reference) {
			return false
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

	tb := bcn.DataPointer

	if tb == nil {
		fmt.Printf("No data attached to block, not updating")
		succeeded = false
		return
	}
	if bcn.PrevBlockChainNode != um.attachedBlockChainNode {
		fmt.Printf("updating with the wrong blockchainnode, not updating")
		return false
	}
	if !bcn.PrevBlockChainNode.UTxOManagerIsUpToDate {
		fmt.Printf("Previous utxo manager was not up to date, stopping ")
	}

	var waitGroup sync.WaitGroup
	waitGroup.Add(int(tb.InputNumber) + int(tb.OutputNumber))

	for i := range tb.InputList {
		txInput := tb.InputList[i]
		go func() {
			tr := txInput.reference
			um.mapMutex.RLock()
			a, ok := um.UtxOMap[tr.BlockHash]
			um.mapMutex.RUnlock()

			if !ok {
				fmt.Printf("UTxO was not in sync, block missing!, utxo corrupted")
				succeeded = false

			}

			um.secondLayerMutexes[tr.Number].RLock()
			_, ok2 := a[tr.Number]
			um.secondLayerMutexes[tr.Number].RUnlock()

			if !ok2 {
				fmt.Printf("UTxO was not in sync, transaction missing!, utxo corrupted")
				succeeded = false
			}

			um.secondLayerMutexes[tr.Number].Lock()
			delete(a, tr.Number)
			um.secondLayerMutexes[tr.Number].Lock()

			waitGroup.Done()
		}()
	}

	um.mapMutex.RLock()
	a, ok := um.UtxOMap[bcn.Hash]
	um.mapMutex.RUnlock()

	if !ok {
		a = make(map[uint8]bool)
		um.mapMutex.Lock()
		um.UtxOMap[bcn.Hash] = a
		um.mapMutex.Unlock()
	}

	for i := range tb.OutputList {
		go func(i uint8) {
			um.secondLayerMutexes[i].Lock()
			a[i] = true
			um.secondLayerMutexes[i].Unlock()

			waitGroup.Done()
		}(uint8(i))
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
	umCopy.UtxOMap = make(map[[32]byte]map[uint8]bool)

	// acquire all locks for second layer
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RLock()
	}

	um.mapMutex.RLock()
	for k, v := range um.UtxOMap {
		wg.Add(1)
		go func(v map[uint8]bool, k [32]byte) {
			newMap := make(map[uint8]bool)
			for k2, v2 := range v {
				newMap[k2] = v2
			}

			umCopy.mapMutex.Lock()
			umCopy.UtxOMap[k] = newMap
			umCopy.mapMutex.Unlock()

			wg.Done()
		}(v, k)
	}
	um.mapMutex.RUnlock()

	wg.Wait()

	// release all locks
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RUnlock()
	}

	return umCopy
}
