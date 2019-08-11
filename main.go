package main

import (
	"fmt"
	"time"
)

// NumberOfMiners is local number of threads competing
const NumberOfMiners = 5

func main() {

	broadcaster := NewBroadcaster("receive Channel")

	Miners := make([]*Miner, NumberOfMiners)

	Miners[0] = BlockChainGenesis(3, broadcaster)

	k := Miners[0].BlockChain.Head.Block.serialize()
	fmt.Printf("%s", k)

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

	End := time.Tick(20000 * time.Millisecond)

	for {
		<-End
		fmt.Printf("##########################################################################")
		Miners[0].Print()
		for i := 1; i < 5; i++ {
			Miners[i].PrintHash()
		}
		fmt.Printf("##########################################################################")

	}

}
