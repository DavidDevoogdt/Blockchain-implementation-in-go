package main

import (
	"fmt"
	bc "project/pkg/blockchain"
	"strconv"

	"gopkg.in/abiosoft/ishell.v2"
)

func main() {

	// ##################
	shell := ishell.New()

	shell.Println("Welcome to davidcoin shell.")
	shell.Print("number of miners to start with: ")
	NumberOfMiners, _ := strconv.Atoi(shell.ReadLine())

	broadcaster := bc.NewBroadcaster("Main network")
	Miners := make([]*bc.Miner, NumberOfMiners)
	Miners[0] = bc.BlockChainGenesis("miner0", 3, broadcaster)
	MinerNames := make([]string, NumberOfMiners)
	MinerNames[0] = "miner0"

	for i := 1; i < NumberOfMiners; i++ {
		mName := fmt.Sprintf("miner%d", i)
		fmt.Printf("seting up miner %s\n", mName)
		Miners[i] = bc.CreateMiner(mName, broadcaster, Miners[0].BlockChain.SerializeBlockChain())
		MinerNames[i] = mName
	}

	for i := 0; i < NumberOfMiners; i++ {
		go Miners[i].MineContiniously()
	}

	shell.AddCmd(&ishell.Cmd{
		Name: "AddMiner",
		Help: "AddMiner -n name -d true",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			c.Print("Name miner: ")
			name := c.ReadLine()

			m := bc.CreateMiner(name, broadcaster, Miners[0].BlockChain.SerializeBlockChain())
			NumberOfMiners++
			Miners = append(Miners, m)
			MinerNames = append(MinerNames, name)

			choice := c.MultiChoice([]string{
				"Miner",
				"Observer",
			}, "Role of miner")
			if choice == 0 {
				go m.MineContiniously()
			}

			choice2 := c.MultiChoice([]string{
				"No",
				"Yes",
			}, "Print every debug message on this shell")
			if choice2 == 1 {
				m.SetDebug(true)
			}
			c.Println("Created Miner.")
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "PrintMiner",
		Help: "PrintMiner",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			choice := c.MultiChoice(MinerNames, "Chose miner")

			Miners[choice].Print()
		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "PrintEveryone",
		Help: "Print every miner Miner",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			fmt.Printf("##########################################################################")

			Miners[0].Print()
			for i := 1; i < NumberOfMiners; i++ {
				Miners[i].PrintHash(3)
			}

			fmt.Printf("##########################################################################")

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "PrintBelongings",
		Help: "Print the current cash status of selected poeple",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			choices := c.Checklist(MinerNames, "select the miners", nil)

			c.Println(choices)
			for _, j := range choices {
				Miners[j].Wallet.Print()
			}

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "PrintBelongingsEveryone",
		Help: "Print the current cash status of all miners",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			for _, m := range Miners {
				m.Wallet.Print()
			}

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "Diagnostics",
		Help: "Print some diagnostics usefull for debugging",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			var am uint64

			for _, m := range Miners {
				k := m.Wallet.TotalUnspent()
				c.Printf("%s has %.4f\n", m.Name, float64(k)/1e18)
				am += k
			}

			n := Miners[0].BlockChain.GetHead().GetBlockNum()

			fmt.Printf("totalBlocks: %d, total money: %.4f", n, float64(am)/1e18)

		},
	})

	shell.AddCmd(&ishell.Cmd{
		Name: "MakeTransaction",
		Help: "make a transaction",
		Func: func(c *ishell.Context) {
			c.ShowPrompt(false)
			defer c.ShowPrompt(true)

			payer := Miners[c.MultiChoice(MinerNames, "Chose Payer:")]
			receiver := Miners[c.MultiChoice(MinerNames, "Chose receiver:")]
			shell.Print("amount: ")
			amount, _ := strconv.ParseFloat(shell.ReadLine(), 64)
			shell.Print("message for transaction: ")
			msg := shell.ReadLine()

			ret := payer.Wallet.MakeTransaction(receiver.Wallet.PublicKey, msg, uint64(amount*1e18))
			if ret {
				c.Printf("transaction of %.4f from %s to %s was succesfull\n", amount, payer.Name, receiver.Name)
			} else {
				c.Printf("transaction of %.4f from %s to %s was unsuccesfull\n", amount, payer.Name, receiver.Name)
			}

		},
	})

	shell.Run()
	/*	NumberOfMiners := 5
		broadcaster := bc.NewBroadcaster("Main network")
		Miners := make([]*bc.Miner, NumberOfMiners)
		Miners[0] = bc.BlockChainGenesis("miner0", 3, broadcaster)
		go Miners[0].MineContiniously()
		Miners[0].StartDebug()
		MinerNames := make([]string, NumberOfMiners)
		MinerNames[0] = "miner0"

		for i := 1; i < NumberOfMiners; i++ {
			mName := fmt.Sprintf("miner%d", i)
			fmt.Printf("seting up miner %s\n", mName)
			Miners[i] = bc.CreateMiner(mName, broadcaster, Miners[0].BlockChain.SerializeBlockChain())
			MinerNames[i] = mName
			Miners[i].StartDebug()
		}

		for i := 0; i < NumberOfMiners; i++ {
			go Miners[i].MineContiniously()
		}

		k := time.Tick(10000 * time.Millisecond)

		for {
			<-k
			fmt.Printf("miner 0 has %d money\n", Miners[0].Wallet.TotalUnspent())
			Miners[0].Wallet.MakeTransaction(Miners[1].Wallet.PublicKey, "yeet", uint64(0.5e18))
		}*/

}
