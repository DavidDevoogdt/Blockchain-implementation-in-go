package main

import (
	"fmt"
	"sync"
	"time"
)

// NumberOfMiners is local number of threads competing

func main() {

	NumberOfMiners := 6

	broadcaster := NewBroadcaster("receive Channel")
	fmt.Printf("made broadcaster\n")
	Miners := make([]*Miner, NumberOfMiners)

	var minersMutex sync.Mutex

	Miners[0] = BlockChainGenesis(3, broadcaster)

	Miners[0].StartDebug()

	fmt.Printf("made genesis miner\n")

	for i := 1; i < NumberOfMiners; i++ {
		fmt.Printf("seting up miner %d\n", i)
		Miners[i] = CreateMiner(fmt.Sprintf("miner%d", i), broadcaster, Miners[0].BlockChain.SerializeBlockChain())
	}

	for i := 0; i < NumberOfMiners; i++ {
		go Miners[i].MineContiniously()
	}

	InsertMiner := time.Tick(1000 * time.Millisecond)

	go func() {
		for {
			<-InsertMiner
			m := MinerFromScratch(fmt.Sprintf("miner%d", NumberOfMiners), broadcaster)

			minersMutex.Lock()
			Miners = append(Miners, m)
			NumberOfMiners++
			n := NumberOfMiners
			minersMutex.Unlock()

			//m.StartDebug()

			if n%2 == 0 {
				go m.MineContiniously()
			}

			if n == 20 {
				return
			}

		}

	}()

	End := time.Tick(10000 * time.Millisecond)

	for {
		<-End

		fmt.Printf("##########################################################################")

		minersMutex.Lock()
		Miners[0].Print()
		for i := 1; i < NumberOfMiners; i++ {
			Miners[i].PrintHash(3)
			//Miners[i].PrintHash(3)
		}
		minersMutex.Unlock()

		fmt.Printf("##########################################################################")

	}

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

	//testFunc()
}

func testFunc() {
	Endfunctions := make([]func(), 0)

	defer func() {
		for _, f := range Endfunctions {
			f()
		}
	}()

	defer fmt.Printf("later defer\n")

	Endfunctions = append(Endfunctions, func() {
		fmt.Printf("heey\n")

	})
}
