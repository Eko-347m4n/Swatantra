package core

import (
	"math/big"
	"time"

	"swatantra/crypto"
)

// CreateGenesisBlock membuat block pertama dalam blockchain.
func CreateGenesisBlock(coinbaseAddr crypto.Address, initialSupply uint64, initialDifficulty uint32) *Block {
	// Transaksi Coinbase untuk genesis block
	coinbaseTx := &Transaction{
		Inputs: []*TxInput{
			// Input pertama untuk coinbase tx memiliki PrevTxHash nol
			{PrevTxHash: crypto.Hash{}, PrevOutIndex: 0, Signature: nil, PublicKey: nil},
		},
		Outputs: []*TxOutput{
			{Value: initialSupply, Address: coinbaseAddr},
		},
	}

	header := &Header{
		Version:        1,
		PrevHash:       crypto.Hash{},
		Height:         0,
		Timestamp:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		Difficulty:     initialDifficulty, // Difficulty awal
		Nonce:          0,  // Nonce akan dicari
		EMABlockTime:   int64(15 * time.Second), // Waktu target block awal
		CumulativeWork: big.NewInt(0),
	}

	block := NewBlock(header, []*Transaction{coinbaseTx})

	// Hitung MerkleRoot
	mTree, _ := NewMerkleTree(block.Transactions)
	block.Header.MerkleRoot = mTree.RootNode.Data

	// Lakukan Proof of Work untuk genesis block
	pow := NewProofOfWork(block)
	nonce, _, err := pow.Run()
	if err != nil {
		// Ini seharusnya tidak terjadi untuk genesis block
		panic(err) 
	}
	block.Header.Nonce = nonce

	return block
}
