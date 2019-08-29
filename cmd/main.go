package main

import (
	"fmt"
	bc "project/pkg/blockchain"
	"sync"
	"time"
)

// NumberOfMiners is local number of threads competing

func main() {

	NumberOfMiners := 6

	broadcaster := bc.NewBroadcaster("Main network")

	fmt.Printf("made broadcaster\n")
	Miners := make([]*bc.Miner, NumberOfMiners)

	var minersMutex sync.Mutex

	Miners[0] = bc.BlockChainGenesis(3, broadcaster)

	Miners[0].StartDebug()

	fmt.Printf("made genesis miner\n")

	for i := 1; i < NumberOfMiners; i++ {
		fmt.Printf("seting up miner %d\n", i)
		Miners[i] = bc.CreateMiner(fmt.Sprintf("miner%d", i), broadcaster, Miners[0].BlockChain.SerializeBlockChain())
	}

	for i := 0; i < NumberOfMiners; i++ {
		go Miners[i].MineContiniously()
	}

	InsertMiner := time.Tick(1000 * time.Millisecond)

	go func() {
		for {
			<-InsertMiner
			m := bc.MinerFromScratch(fmt.Sprintf("miner%d", NumberOfMiners), broadcaster)

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
}
