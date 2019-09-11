Davidcoin
=======


This is a implementation of a proof of work (POW) cryptocoin in go intended as a hobby project. The project runs concurrently a number of miners, each of which keeps its own copy of the blockchain. The miners are able to communicate with each other through a single channel. Through this nework, mined blocks are broadcast, transactionrequest are made, unknown blocks for new branches are requested,... An easy to use shell can be used to see the blockchain in action and interact with it.

How to use
-------

git clone this repo, run:
```
go run cmd/main.go
```
Follow the instructions in the shell for more information. Type help for overview of the available commands

Under the hood
--------
The internals of this project can be found in the paper.pdf file in the root of the repository

Images
----------
Preview of the shell inteface provided with the program:
![shell image](/Images/ShellExample.png)

Overview of the concurrency patterns used in the program. For more information, read the pdf.
![concurrency patterns](/Images/ConcurrencyPattern.svg)
