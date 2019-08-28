package main

import (
	"fmt"
	"sync"

	"github.com/sasha-s/go-deadlock"
)

// UTxOManager keeps track of unspent transaction outputs (utxo)
type UTxOManager struct {
	attachedBlockChainNode *BlockChainNode
	Miner                  *Miner

	umMutex deadlock.RWMutex

	UtxOMap            map[[32]byte]map[uint8]map[uint8]bool //merkleroot -> tx number -> output number
	mapMutex           sync.RWMutex
	secondLayerMutexes [256]sync.RWMutex
	thirdLayerMutexes  [256]sync.RWMutex //to protect all the nested maps, a common set of mutexes is used irrespective of the block hash. This is lightweight and still faster than using a single mutex for all

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

	bcn.generalMutex.RLock()
	_prevBCN := bcn.PrevBlockChainNode
	tbg := bcn.DataPointer
	bcn.generalMutex.RUnlock()

	um.umMutex.RLock()
	_attachedBCN := um.attachedBlockChainNode
	_miner := um.Miner
	um.umMutex.RUnlock()

	var umn *UTxOManager
	_ = umn
	if deepCopy {
		wgDeepCopy.Add(1)
		go func() {
			umn = um.Deepcopy()
			wgDeepCopy.Done()
		}()

	}

	if tbg == nil {
		_miner.DebugPrint(fmt.Sprintf("No data attached to block, not updating"))
		succeeded = false
		return
	}
	if _prevBCN == nil {
		_miner.DebugPrint(fmt.Sprintf("bcn has no pref blockchainnode, returning"))
		return false
	}

	if _prevBCN != _attachedBCN {
		_miner.DebugPrint(fmt.Sprintf("updating with the wrong blockchainnode, not updating"))
		return false
	}

	_prevBCN.generalMutex.RLock()

	if !bcn.PrevBlockChainNode.UTxOManagerIsUpToDate {
		_miner.DebugPrint(fmt.Sprintf("Previous utxo manager was not up to date, stopping "))
	}
	_prevBCN.generalMutex.RUnlock()

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
						_miner.DebugPrint(fmt.Sprintf("UTxO was not in sync, merkle missing!, utxo corrupted"))
						succeeded = false

					}

					um.secondLayerMutexes[tr.OutputNumber].RLock()
					b, ok2 := a[tr.TransactionBlockNumber]
					um.secondLayerMutexes[tr.OutputNumber].RUnlock()

					if !ok2 {
						_miner.DebugPrint(fmt.Sprintf("UTxO was not in sync, transaction block missing!, utxo corrupted"))
						succeeded = false
					}

					um.thirdLayerMutexes[tr.OutputNumber].RLock()
					_, ok3 := b[tr.OutputNumber]
					um.thirdLayerMutexes[tr.OutputNumber].RUnlock()

					if !ok3 {
						_miner.DebugPrint(fmt.Sprintf("UTxO was not in sync, transaction output missing!, utxo corrupted"))
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
		_miner.DebugPrint(fmt.Sprintf("Some record could not be correctly updated in the map, probably missing"))
		return
	}

	um.umMutex.Lock()
	um.attachedBlockChainNode = bcn
	_attachedBCN = bcn
	um.umMutex.Unlock()

	_prevBCN.generalMutex.Lock()
	if deepCopy {
		_prevBCN.UTxOManagerIsUpToDate = true
		_prevBCN.generalMutex.Unlock()
		wgDeepCopy.Wait() //was done async, needs to be done now. Lock released for speed

		_prevBCN.generalMutex.Lock()
		_prevBCN.UTxOManagerPointer = umn
		umn.attachedBlockChainNode = _prevBCN
	} else {
		bcn.PrevBlockChainNode.UTxOManagerIsUpToDate = false
		bcn.PrevBlockChainNode.UTxOManagerPointer = nil

	}
	_prevBCN.generalMutex.Unlock()

	_attachedBCN.generalMutex.Lock()
	_attachedBCN.UTxOManagerIsUpToDate = true
	_attachedBCN.UTxOManagerPointer = um
	_attachedBCN.generalMutex.Unlock()

	return true
}

// Deepcopy generates exact copy at different memory location
func (um *UTxOManager) Deepcopy() *UTxOManager {
	var wg sync.WaitGroup
	umCopy := new(UTxOManager)

	um.umMutex.Lock()
	umCopy.attachedBlockChainNode = um.attachedBlockChainNode
	um.umMutex.Unlock()

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
