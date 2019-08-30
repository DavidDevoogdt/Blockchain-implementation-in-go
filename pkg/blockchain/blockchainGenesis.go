package davidcoin

// BlockChainGenesis creates first miner, mines first block and initiates the blockchain
func BlockChainGenesis(name string, difficulty uint32, broadcaster *Broadcaster) *Miner {

	bl := new(BlockChainNode)
	bl.PrevBlockChainNode = bl
	bl.dataHasArrived = make(chan bool)

	bc := new(BlockChain)
	bc.AllNodesMap = make(map[[32]byte]*BlockChainNode)
	bc.OtherHeadNodes = make(map[[32]byte]*BlockChainNode)
	bc.OrphanBlockChains = make([]*BlockChain, 0)
	bc.IsOrphan = false
	bc.Root = bl

	bc.utxoChan = make(chan *BlockChainNode)

	genesisMiner := CreateGenesisMiner(name, broadcaster, bc)
	bc.Miner = genesisMiner
	gen := generateGenesis(difficulty, genesisMiner)
	bl.Block = gen
	bl.Hash = bl.Block.Hash()

	bc.Head = bl
	bc.Miner.Debug = false
	bc.AllNodesMap[bl.Hash] = bl
	bc.DanglingData = make(map[[32]byte]*TransactionBlockGroup)
	bc.MerkleRootToBlockHashMap = make(map[[32]byte][32]byte)

	bl.HasData = false
	bl.UTxOManagerIsUpToDate = true
	bl.UTxOManagerPointer = InitializeUTxOMananger(genesisMiner)

	bl.dataHasArrived = make(chan bool)

	go bc.utxoUpdater()

	return genesisMiner
}
