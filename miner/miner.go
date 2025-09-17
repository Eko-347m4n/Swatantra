package miner

import (
	"fmt"
	"time"

	"swatantra/core"
	"swatantra/crypto"
	"swatantra/mempool"
	"swatantra/p2p"
)

type Miner struct {
	blockchain   *core.Blockchain
	mempool      *mempool.Mempool
	server       *p2p.Server
	coinbase     crypto.Address // The address to receive mining rewards
	maxBlockSize int
}

func NewMiner(bc *core.Blockchain, mp *mempool.Mempool, srv *p2p.Server, coinbase crypto.Address, maxBlockSize int) *Miner {
	return &Miner{
		blockchain:   bc,
		mempool:      mp,
		server:       srv,
		coinbase:     coinbase,
		maxBlockSize: maxBlockSize,
	}
}

func (m *Miner) Start() {
	fmt.Println("Starting miner...")
	go m.loop()
}

func (m *Miner) loop() {
	for {
		block, err := m.createNewBlock()
		if err != nil {
			fmt.Println("Error creating new block:", err)
			time.Sleep(2 * time.Second)
			continue
		}

		pow := core.NewProofOfWork(block)
		nonce, hash, err := pow.Run()
		if err != nil {
			fmt.Println("Error running proof of work:", err)
			continue
		}

		block.Header.Nonce = nonce
		
		fmt.Printf("Mined new block! hash: %s, nonce: %d, height: %d, txs: %d\n", hash.ToHex(), nonce, block.Header.Height, len(block.Transactions))

		if err := m.blockchain.AddBlock(block); err != nil {
			fmt.Println("Error adding mined block to blockchain:", err)
			continue
		}

		if err := m.server.BroadcastBlock(block); err != nil {
			fmt.Println("Error broadcasting mined block:", err)
		}
	}
}

func (m *Miner) createNewBlock() (*core.Block, error) {
	parentHeader := m.blockchain.Head()
	
	// Get transactions from mempool
	txs := m.mempool.GetTransactions(m.maxBlockSize) // Use GetTransactions
	
	// Create coinbase transaction
	coinbaseTx := core.NewTransaction(
		[]*core.TxInput{{PrevTxHash: crypto.Hash{}, PrevOutIndex: 0}},
		[]*core.TxOutput{{Value: 50, Address: m.coinbase}}, // Reward of 50
	)
	
	allTxs := append([]*core.Transaction{coinbaseTx}, txs...)

	// TODO: Check block size limit

	merkleTree, err := core.NewMerkleTree(allTxs)
	if err != nil {
		return nil, err
	}

	// --- FIX: Call time.Now() only once ---
	newTimestamp := time.Now().UnixNano()

	// Calculate next difficulty
	difficulty, emaBlockTime := m.blockchain.CalculateNextDifficulty(parentHeader, newTimestamp)

	header := &core.Header{
		Version:      1,
		PrevHash:     parentHeader.Hash(),
		Height:       parentHeader.Height + 1,
		Timestamp:    newTimestamp, // Use the stored timestamp
	
MerkleRoot:   merkleTree.RootNode.Data,
		Difficulty:   difficulty,
		EMABlockTime: emaBlockTime,
	}

	fmt.Printf("createNewBlock: Parent Hash (from blockchain.Head()): %s\n", parentHeader.Hash().ToHex())
	fmt.Printf("createNewBlock: New Block PrevHash: %s\n", header.PrevHash.ToHex())

	return core.NewBlock(header, allTxs), nil
}
