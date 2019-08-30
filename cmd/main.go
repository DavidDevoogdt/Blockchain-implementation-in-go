package main

import (
	"fmt"
	bc "project/pkg/blockchain"
	"time"

	"github.com/sasha-s/go-deadlock"
	"gopkg.in/abiosoft/ishell.v2"
)

func main() {
	fmt.Print("press enter: ")
	var input string
	fmt.Scanln(&input)
	if input != "debug" {

		NumberOfMiners := 1
		broadcaster := bc.NewBroadcaster("Main network")
		Miners := make([]*bc.Miner, 1)
		Miners[0] = bc.BlockChainGenesis("Genesis", 3, broadcaster)
		MinerNames := make([]string, 1)
		MinerNames[0] = "Genesis"

		// ##################
		shell := ishell.New()

		shell.Println("Welcome to davidcoin shell. start by typing help")

		shell.AddCmd(&ishell.Cmd{
			Name: "AddMiner",
			Help: "AddMiner -n name -d true",
			Func: func(c *ishell.Context) {
				c.ShowPrompt(false)
				defer c.ShowPrompt(true) // yes, revert after login.

				// get username
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
					m.StartDebug()
				}
				c.Println("Created Miner.")
			},
		})

		shell.AddCmd(&ishell.Cmd{
			Name: "PrintMiner",
			Help: "PrintMiner",
			Func: func(c *ishell.Context) {
				c.ShowPrompt(false)
				defer c.ShowPrompt(true) // yes, revert after login.

				choice := c.MultiChoice(MinerNames, "Chose miner")

				Miners[choice].Print()
			},
		})

		shell.AddCmd(&ishell.Cmd{
			Name: "PrintEveryone",
			Help: "Print every miner Miner",
			Func: func(c *ishell.Context) {
				c.ShowPrompt(false)
				defer c.ShowPrompt(true) // yes, revert after login.

				fmt.Printf("##########################################################################")

				Miners[0].Print()
				for i := 1; i < NumberOfMiners; i++ {
					Miners[i].PrintHash(3)
				}

				fmt.Printf("##########################################################################")

			},
		})

		shell.Run()

	} else {

		NumberOfMiners := 6

		broadcaster := bc.NewBroadcaster("Main network")
		Miners := make([]*bc.Miner, NumberOfMiners)

		fmt.Printf("made broadcaster\n")

		var minersMutex deadlock.Mutex

		Miners[0] = bc.BlockChainGenesis("Genesis", 3, broadcaster)

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
			go func() {
				fmt.Printf("##########################################################################")

				minersMutex.Lock()
				Miners[0].Print()
				for i := 1; i < NumberOfMiners; i++ {
					Miners[i].PrintHash(3)
					//Miners[i].PrintHash(3)
				}
				minersMutex.Unlock()

				fmt.Printf("##########################################################################")

			}()

		}
	}
}
