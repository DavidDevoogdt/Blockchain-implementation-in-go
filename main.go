package main

import (
	"fmt"
	"time"
)

// NumberOfMiners is local number of threads competing

func main() {

	NumberOfMiners := 1

	broadcaster := NewBroadcaster("receive Channel")
	fmt.Printf("made broadcaster\n")
	Miners := make([]*Miner, NumberOfMiners)

	Miners[0] = BlockChainGenesis(3, broadcaster)

	fmt.Printf("made genesis miner\n")

	//blc := Miners[0].MineBlock("genblock1")

	//fmt.Printf("mined first block\n")

	//Miners[0].BlockChain.addBlockChainNode(blc)
	//Miners[0].BroadcastBlock(blc)

	for i := 1; i < NumberOfMiners; i++ {
		fmt.Printf("seting up miner %d\n", i)
		Miners[i] = CreateMiner(fmt.Sprintf("miner%d", i), broadcaster, Miners[0].BlockChain.SerializeBlockChain())
	}

	Miners[0].StartDebug()

	for i := 0; i < NumberOfMiners; i++ {
		go Miners[i].MineContiniously()
	}

	InsertMiner := time.Tick(10000 * time.Millisecond)

	go func() {
		for {
			<-InsertMiner
			m := MinerFromScratch(fmt.Sprintf("miner%d", NumberOfMiners), broadcaster)

			//m.StartDebug()
			Miners = append(Miners, m)

			//go m.MineContiniously()

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
		for i := 1; i < NumberOfMiners; i++ {
			Miners[i].PrintHash(3)
		}
		fmt.Printf("##########################################################################")

	}
}
