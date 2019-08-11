This is a simple implementation of a blockchain in go intended as a hobby project
Each miner generates its own blockchain in memory. New blocks are shared amogst the miners via go channels. Blocks can be requested by seperate channels. This is a proof of work based (PoW) blockchain. Some critical features are not implemented yet
