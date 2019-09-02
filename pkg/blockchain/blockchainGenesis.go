package davidcoin

// BlockChainGenesis creates first miner, mines first block and initiates the blockchain
func BlockChainGenesis(name string, difficulty uint32, broadcaster *Broadcaster) *Miner {

	bl := new(BlockChainNode)
	bl.previousBlockChainNode = bl
	bl.dataHasArrived = make(chan bool)

	bc := new(BlockChain)
	bc.allNodesMap = make(map[[32]byte]*BlockChainNode)
	bc.otherHeadNodes = make(map[[32]byte]*BlockChainNode)
	bc.orphanBlockChains = make([]*BlockChain, 0)
	bc.isOrphan = false
	bc.root = bl

	bc.utxoChan = make(chan *BlockChainNode)

	genesisMiner := createGenesisMiner(name, broadcaster, bc)
	bc.Miner = genesisMiner
	gen := generateGenesis(difficulty, genesisMiner)
	bl.Block = gen
	bl.Hash = bl.Block.Hash()

	bc.head = bl
	bc.Miner.debug = false
	bc.allNodesMap[bl.Hash] = bl
	bc.danglingData = make(map[[32]byte]*transactionBlockGroup)
	bc.merkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	bl.hasData = false
	bl.uTxOManagerIsUpToDate = true
	bl.uTxOManagerPointer = initializeUTxOMananger(genesisMiner)

	bl.dataHasArrived = make(chan bool)

	go bc.utxoUpdater()

	return genesisMiner
}
