Davidcoin
=======


This is a implementation of a proof of work (POW) cryptocoin in go intended as a hobby project. The project runs concurrently a number of miners, each of which keep a copy of the blockchain. The miners are able to communicate with each other through a channel which accepts a byte arrays. Through this nework, mined blocks are broadcast, transactionrequest, previously missed blocks,... 
An easy to use shell can be used to see the blockchain in action and interact with it

How to install
-------
git clone this repo, run:´
```
go run cmd/main.go
```


Under the hood
--------
The internals of this project can be found in the paper.pdf file in the root of the repository
