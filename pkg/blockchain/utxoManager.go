package davidcoin

import (
	"fmt"
	"sync"

	"github.com/sasha-s/go-deadlock"
)

// uTxOManager keeps track of unspent transaction outputs (utxo)
type uTxOManager struct {
	attachedBlockChainNode *BlockChainNode
	Miner                  *Miner

	umMutex deadlock.RWMutex

	utxOMap            map[[32]byte]map[uint8]map[uint8]bool //blockhash -> tx block number -> output number
	mapMutex           sync.RWMutex
	secondLayerMutexes [256]sync.RWMutex
	thirdLayerMutexes  [256]sync.RWMutex //to protect all the nested maps, a common set of mutexes is used irrespective of the block hash. This is lightweight and still faster than using a single mutex for all

}

// initializeUTxOMananger is used to initialize the blockchain
func initializeUTxOMananger(m *Miner) *uTxOManager {
	utxo := new(uTxOManager)
	utxo.utxOMap = make(map[[32]byte]map[uint8]map[uint8]bool)
	utxo.Miner = m
	utxo.attachedBlockChainNode = m.BlockChain.Root
	utxo.attachedBlockChainNode.hasData = false
	utxo.attachedBlockChainNode.uTxOManagerIsUpToDate = true
	return utxo
}

// verifyTransactionRef checks whether a transactionref is spendable
func (um *uTxOManager) verifyTransactionRef(tr *transactionRef) bool {
	um.mapMutex.RLock()
	a, ok := um.utxOMap[tr.BlockHash]
	um.mapMutex.RUnlock()

	if !ok {
		um.Miner.DebugPrint(fmt.Sprintf("Transaction block has root not in chain, invalid transaction: %x - %d %d \n", tr.BlockHash, tr.TransactionBlockNumber, tr.OutputNumber))
		return false
	}

	um.secondLayerMutexes[tr.OutputNumber].RLock()
	b, ok2 := a[tr.TransactionBlockNumber]
	um.secondLayerMutexes[tr.OutputNumber].RUnlock()

	if !ok2 {
		fmt.Printf("Transaction block number not in chain, invalid transaction\n")
		return false
	}

	um.thirdLayerMutexes[tr.OutputNumber].RLock()
	c, ok3 := b[tr.OutputNumber]
	um.thirdLayerMutexes[tr.OutputNumber].RUnlock()

	if !ok3 {
		um.Miner.DebugPrint(fmt.Sprintf("Transaction already spent or does not exist\n"))
		return false
	}
	return c // still could be false
}

// VerifyTransactionBlockRefs only checks whether or not the transactions in the inputlist are spendable, not the signature and such
func (um *uTxOManager) VerifyTransactionBlockRefs(tb *transactionBlock) bool {

	for _, b := range tb.InputList[:] {

		if !um.verifyTransactionRef(b.reference) {
			return false
		}
	}

	return true
}

// verifyTransactionBlockGroupRefs only checks whether or not the transactions in the inputlist are spendable, not the signature and such
func (um *uTxOManager) verifyTransactionBlockGroupRefs(tbg *transactionBlockGroup) bool {

	for _, tb := range tbg.TransactionBlockStructs {
		for _, b := range tb.InputList[:] {

			if !um.verifyTransactionRef(b.reference) {
				return false
			}
		}
	}
	return true
}

// updateWithNextBlockChainNode adds new data to map. Deepcopy preserves map to previous block
func (um *uTxOManager) updateWithNextBlockChainNode(bcn *BlockChainNode, deepCopy bool) (succeeded bool) {
	succeeded = true

	bcn.generalMutex.RLock()
	_prevBCN := bcn.previousBlockChainNode
	tbg := bcn.dataPointer
	bcn.generalMutex.RUnlock()

	um.umMutex.RLock()
	_attachedBCN := um.attachedBlockChainNode
	_miner := um.Miner
	um.umMutex.RUnlock()

	var umn *uTxOManager
	_ = umn
	if deepCopy {
		umn = um.Deepcopy() // cannot be done async because the state would be changed
	}

	if tbg == nil {
		_miner.DebugPrint(fmt.Sprintf("No data attached to block, not updating\n"))
		succeeded = false
		return
	}
	if _prevBCN == nil {
		_miner.DebugPrint(fmt.Sprintf("bcn has no pref blockchainnode, returning\n"))
		return false
	}

	if _prevBCN != _attachedBCN {
		_miner.DebugPrint(fmt.Sprintf("updating with the wrong blockchainnode, not updating\n"))
		return false
	}

	_prevBCN.generalMutex.RLock()

	if !bcn.previousBlockChainNode.uTxOManagerIsUpToDate {
		_miner.DebugPrint(fmt.Sprintf("Previous utxo manager was not up to date, stopping \n"))
	}
	_prevBCN.generalMutex.RUnlock()

	var waitGroup sync.WaitGroup

	// update every transactionBlock Async
	for k, tb := range tbg.TransactionBlockStructs {
		waitGroup.Add(int(tb.InputNumber) + int(tb.OutputNumber))
		//go func(tb *TransactionBlock, k int) {
		for i := range tb.InputList {
			txInput := tb.InputList[i]
			//go func() {
			tr := txInput.reference
			um.mapMutex.RLock()
			a, ok := um.utxOMap[tr.BlockHash]
			um.mapMutex.RUnlock()

			if !ok {
				_miner.DebugPrint(fmt.Sprintf("UTxO was not in sync, hash block missing!, utxo corrupted\n"))
				succeeded = false
			}

			um.secondLayerMutexes[tr.TransactionBlockNumber].RLock()
			b, ok2 := a[tr.TransactionBlockNumber]
			um.secondLayerMutexes[tr.TransactionBlockNumber].RUnlock()

			if !ok2 {
				_miner.DebugPrint(fmt.Sprintf("UTxO was not in sync, transaction block missing!, utxo corrupted\n"))
				succeeded = false
			}

			um.thirdLayerMutexes[tr.OutputNumber].RLock()
			_, ok3 := b[tr.OutputNumber]
			um.thirdLayerMutexes[tr.OutputNumber].RUnlock()

			if !ok3 {
				_miner.DebugPrint(fmt.Sprintf("UTxO was not in sync, transaction output missing!, utxo corrupted\n"))
				succeeded = false
			}

			um.thirdLayerMutexes[tr.OutputNumber].Lock()
			delete(b, tr.OutputNumber)
			um.thirdLayerMutexes[tr.OutputNumber].Unlock()

			waitGroup.Done()
			//}()
		}

		um.mapMutex.RLock()
		a, ok := um.utxOMap[bcn.Hash]
		um.mapMutex.RUnlock()

		if !ok {
			a = make(map[uint8]map[uint8]bool)
			um.mapMutex.Lock()
			um.utxOMap[bcn.Hash] = a
			um.mapMutex.Unlock()
		}

		uk := uint8(k)

		um.secondLayerMutexes[uk].RLock()
		b, ok2 := a[uk]
		um.secondLayerMutexes[uk].RUnlock()

		if !ok2 {
			b = make(map[uint8]bool)

			um.secondLayerMutexes[uk].Lock()
			a[uk] = b
			um.secondLayerMutexes[uk].Unlock()
		}

		for j := range tb.OutputList {

			um.thirdLayerMutexes[j].Lock()
			b[uint8(j)] = true
			um.thirdLayerMutexes[j].Unlock()
			waitGroup.Done()

		}

		//}(tb, k)
	}

	waitGroup.Wait()

	if !succeeded {
		_miner.DebugPrint(fmt.Sprintf("Some record could not be correctly updated in the map, probably missing\n"))
		return
	}

	um.umMutex.Lock()
	um.attachedBlockChainNode = bcn
	_attachedBCN = bcn
	um.umMutex.Unlock()

	_prevBCN.generalMutex.Lock()
	if deepCopy {
		_prevBCN.uTxOManagerIsUpToDate = true
		_prevBCN.uTxOManagerPointer = umn
		umn.attachedBlockChainNode = _prevBCN
	} else {
		bcn.previousBlockChainNode.uTxOManagerIsUpToDate = false
		bcn.previousBlockChainNode.uTxOManagerPointer = nil

	}
	_prevBCN.generalMutex.Unlock()

	_attachedBCN.generalMutex.Lock()
	_attachedBCN.uTxOManagerIsUpToDate = true
	_attachedBCN.uTxOManagerPointer = um
	_attachedBCN.generalMutex.Unlock()

	return true
}

// Deepcopy generates exact copy at different memory location
func (um *uTxOManager) Deepcopy() *uTxOManager {
	var wg sync.WaitGroup
	umCopy := new(uTxOManager)

	um.umMutex.Lock()
	umCopy.attachedBlockChainNode = um.attachedBlockChainNode
	um.umMutex.Unlock()

	umCopy.utxOMap = make(map[[32]byte]map[uint8]map[uint8]bool)

	// acquire all locks for second layer
	for i := range um.secondLayerMutexes {
		um.secondLayerMutexes[i].RLock()
	}

	// acquire all locks for third layer
	for i := range um.thirdLayerMutexes {
		um.thirdLayerMutexes[i].RLock()
	}

	um.mapMutex.RLock()
	for k, v := range um.utxOMap {
		wg.Add(1)

		go func(v map[uint8]map[uint8]bool, k [32]byte) {
			newMap := make(map[uint8]map[uint8]bool)
			var newMapMutex sync.Mutex
			for k2, v2 := range v {
				wg.Add(1)

				go func(v2 map[uint8]bool, k2 uint8) {
					newMap2 := make(map[uint8]bool)
					for k3, v3 := range v2 {
						newMap2[k3] = v3
					}

					newMapMutex.Lock()
					newMap[k2] = newMap2
					newMapMutex.Unlock()

					wg.Done()
				}(v2, k2)
			}
			umCopy.mapMutex.Lock()
			umCopy.utxOMap[k] = newMap
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
func (um *uTxOManager) Print() {

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
	for k, v := range um.utxOMap {
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
