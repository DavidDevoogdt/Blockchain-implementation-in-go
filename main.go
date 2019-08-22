package main

// NumberOfMiners is local number of threads competing

func main() {

	/*NumberOfMiners := 1

	broadcaster := NewBroadcaster("receive Channel")
	fmt.Printf("made broadcaster\n")
	Miners := make([]*Miner, NumberOfMiners)

	Miners[0] = BlockChainGenesis(3, broadcaster)

	fmt.Printf("made genesis miner\n")

	for i := 1; i < NumberOfMiners; i++ {
		fmt.Printf("seting up miner %d\n", i)
		Miners[i] = CreateMiner(fmt.Sprintf("miner%d", i), broadcaster, Miners[0].BlockChain.SerializeBlockChain())
	}

	//Miners[0].StartDebug()

	for i := 0; i < NumberOfMiners; i++ {
		go Miners[i].MineContiniously()
	}

	InsertMiner := time.Tick(10000 * time.Millisecond)

	go func() {
		for {
			<-InsertMiner
			m := MinerFromScratch(fmt.Sprintf("miner%d", NumberOfMiners), broadcaster)
			Miners = append(Miners, m)

			NumberOfMiners++

			if NumberOfMiners == 7 {
				return
			}

		}

	}()

	End := time.Tick(10000 * time.Millisecond)

	for {
		<-End
		fmt.Printf("##########################################################################")
		Miners[0].Print()
		Miners[0].BlockChain.Head.UTxOManagerPointer.Print()

		for i := 1; i < NumberOfMiners; i++ {
			Miners[i].PrintHash(3)
		}
		fmt.Printf("##########################################################################")

	} */

	/*
		mt := InitializeMerkleTree()

		for i := 0; i < 256; i++ {
			token := make([]byte, 4)
			rand.Read(token)
			mt.Add(&token)
		}

		mt.FinalizeTree()

		hash := mt.levelHash[0][0]

		mp := mt.GenerareteMerkleProof(2)

		fmt.Printf("%t\n", mp.VerifyProofStruct(hash))
	*/
}
