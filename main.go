package main

import (
	"fmt"
	"time"
)

// NumberOfMiners is local number of threads competing

func main() {

	NumberOfMiners := 5

	broadcaster := NewBroadcaster("receive Channel")

	Miners := make([]*Miner, NumberOfMiners)

	Miners[0] = BlockChainGenesis(3, broadcaster)

	blc := Miners[0].MineBlock("genblock1")

	Miners[0].BlockChain.addBlockChainNode(blc)
	Miners[0].BroadcastBlock(blc)

	for i := 1; i < NumberOfMiners; i++ {
		fmt.Printf("seting up miner %d\n", i)
		Miners[i] = CreateMiner(fmt.Sprintf("miner%d", i), broadcaster, Miners[0].BlockChain.SerializeBlockChain())
	}

	for i := 0; i < NumberOfMiners; i++ {
		go Miners[i].MineContiniously()
	}

	InsertMiner := time.After(10000 * time.Millisecond)

	go func() {
		<-InsertMiner
		m := MinerFromScratch(fmt.Sprintf("miner%d", NumberOfMiners), broadcaster)

		Miners = append(Miners, m)

		go m.MineContiniously()
		NumberOfMiners++
	}()

	End := time.Tick(5000 * time.Millisecond)

	for {
		<-End
		fmt.Printf("##########################################################################")
		Miners[0].Print()
		for i := 1; i < NumberOfMiners; i++ {
			Miners[i].PrintHash()
		}
		fmt.Printf("##########################################################################")

	}

	/*
		go Miners[0].MineContiniously()

		Wait := time.After(5000 * time.Millisecond)
		<-Wait


		Miners[1] = MinerFromScratch("David", broadcaster)
		go Miners[1].MineContiniously()

		End := time.Tick(5000 * time.Millisecond)

		for {
			<-End
			fmt.Printf("##########################################################################")
			Miners[0].Print()
			Miners[1].Print()

		}
	*/

}
