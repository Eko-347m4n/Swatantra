package core

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"swatantra/crypto"
	"swatantra/storage"
)

// Helper function to create a simple blockchain for testing
func newTestBlockchain(t *testing.T) (*Blockchain, crypto.PrivateKey) {
	// Create a temporary directory for LevelDB
	tmpDir, err := ioutil.TempDir("", "test_blockchain_db")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Create a new LevelDB store
	store, err := storage.NewLevelDBStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create LevelDB store: %v", err)
	}

	// Clean up the temporary directory and close the store after tests
	t.Cleanup(func() {
		store.Close()
		os.RemoveAll(tmpDir)
	})

	privKey, _ := crypto.GeneratePrivateKey()
	
	// Create genesis block parameters
	initialDifficulty := uint32(10)

	// NewBlockchain only takes store and initialDifficulty
	bc, err := NewBlockchain(store, initialDifficulty)
	if err != nil {
		t.Fatalf("Failed to create test blockchain: %v", err)
	}

	// NewBlockchain should create the genesis block if it doesn't exist.
	// We need to ensure the blockchain's head is set after NewBlockchain.
	// The genesis block's coinbase transaction is created within NewBlockchain.

	return bc, privKey
}

func TestValidateTransaction(t *testing.T) {
	bc, privKey := newTestBlockchain(t)
	defer bc.store.Close()

	// Get the initial UTXO from the genesis block
	genesisBlock, err := bc.GetBlockByHash(bc.Head().Hash())
	if err != nil {
		t.Fatalf("Failed to get genesis block: %v", err)
	}
	coinbaseTx := genesisBlock.Transactions[0]
	
	coinbaseTxHash, err := coinbaseTx.Hash()
	if err != nil {
		t.Fatalf("Failed to get coinbase transaction hash: %v", err)
	}

	initialUTXO := &SpentUTXO{
		TxHash: coinbaseTxHash,
		Index:  0,
		Output: coinbaseTx.Outputs[0],
	}

	// Create a valid transaction
	toPrivKey, _ := crypto.GeneratePrivateKey()
	toAddress := toPrivKey.Public().Address()

	input := &TxInput{
		PrevTxHash:   initialUTXO.TxHash,
		PrevOutIndex: initialUTXO.Index,
		PublicKey:    privKey.Public(),
	}
	output := &TxOutput{
		Value:   500,
		Address: toAddress,
	}
	changeOutput := &TxOutput{
		Value:   499, // 1 for fee
		Address: privKey.Public().Address(),
	}

	tx := NewTransaction([]*TxInput{input}, []*TxOutput{output, changeOutput})
	if err := tx.Sign(privKey); err != nil {
		t.Fatalf("Failed to sign transaction: %v", err)
	}

	valid, err := bc.ValidateTransaction(tx)
	if err != nil {
		t.Errorf("Test 1 (Valid transaction): ValidateTransaction failed: %v", err)
	}
	if !valid {
		t.Error("Test 1 (Valid transaction): Valid transaction was marked as invalid")
	}

	// --- Setup for Double Spend Test ---
	// To test double spend, we need to apply the first transaction (tx) to the blockchain
	// so its input UTXO becomes spent.
	
	// Create a dummy block to include the first transaction (tx)
	dummyHeader := &Header{
		Version:    1,
		PrevHash:   bc.Head().Hash(),
		Height:     bc.Head().Height + 1,
		Timestamp:  time.Now().Unix(),
		Difficulty: bc.Head().Difficulty,
		Nonce:      0, // Will be mined later
	}
	
	// Calculate expected EMABlockTime for the dummy block
	_, expectedEMABlockTime := bc.CalculateNextDifficulty(bc.Head(), dummyHeader.Timestamp)
	dummyHeader.EMABlockTime = expectedEMABlockTime

	dummyBlock := NewBlock(dummyHeader, []*Transaction{tx})
	
	// Calculate MerkleRoot for the dummy block
	mTree, err := NewMerkleTree(dummyBlock.Transactions)
	if err != nil {
		t.Fatalf("Failed to create Merkle tree for dummy block: %v", err)
	}
	dummyBlock.Header.MerkleRoot = mTree.RootNode.Data

	// Mine the dummy block to get a valid PoW
	pow := NewProofOfWork(dummyBlock)
	nonce, _, err := pow.Run()
	if err != nil {
		t.Fatalf("Failed to mine dummy block: %v", err)
	}
	dummyBlock.Header.Nonce = nonce
	
	// Add the dummy block to the blockchain. This will mark the UTXO spent by 'tx'.
	if err := bc.AddBlock(dummyBlock); err != nil {
		t.Fatalf("Failed to add dummy block to blockchain: %v", err)
	}

	// Test 2: Double spend attempt
	// Try to spend the same UTXO again. This should now fail.
	doubleSpendTx := NewTransaction([]*TxInput{input}, []*TxOutput{output})
	if err := doubleSpendTx.Sign(privKey); err != nil {
		t.Fatalf("Failed to sign double spend transaction: %v", err)
	}
	
	valid, err = bc.ValidateTransaction(doubleSpendTx)
	if err == nil {
		t.Error("Test 2 (Double spend): Double spend transaction was not detected, but should have failed with an error")
	}
	if valid {
		t.Error("Test 2 (Double spend): Double spend transaction was marked as valid, but should be invalid")
	}

	// Test: Invalid signature
	invalidSigTx := NewTransaction([]*TxInput{input}, []*TxOutput{output, changeOutput})
	// Tamper with the signature using a fixed size
	invalidSigTx.Inputs[0].Signature = bytes.Repeat([]byte{0x01}, crypto.SignatureSize)
	valid, err = bc.ValidateTransaction(invalidSigTx)
	if err == nil || valid {
		t.Error("Transaction with invalid signature was not detected")
	}

	// Test: Insufficient funds (sum of inputs < sum of outputs)
	insufficientFundsTx := NewTransaction([]*TxInput{input}, []*TxOutput{
		{Value: 1500, Address: toAddress},
	})
	if err := insufficientFundsTx.Sign(privKey); err != nil {
		t.Fatalf("Failed to sign insufficient funds transaction: %v", err)
	}
	valid, err = bc.ValidateTransaction(insufficientFundsTx)
	if err == nil || valid {
		t.Error("Transaction with insufficient funds was not detected")
	}
}

func TestValidateBlock(t *testing.T) {
	bc, privKey := newTestBlockchain(t)
	defer bc.store.Close()

	// Get the initial UTXO from the genesis block
	genesisBlock, err := bc.GetBlockByHash(bc.Head().Hash())
	if err != nil {
		t.Fatalf("Failed to get genesis block: %v", err)
	}
	coinbaseTx := genesisBlock.Transactions[0]
	coinbaseTxHash, err := coinbaseTx.Hash()
	if err != nil {
		t.Fatalf("Failed to get coinbase transaction hash: %v", err)
	}

	initialUTXO := &SpentUTXO{
		TxHash: coinbaseTxHash,
		Index:  0,
		Output: coinbaseTx.Outputs[0],
	}

	// Create a valid transaction to be included in blocks
	toPrivKey, _ := crypto.GeneratePrivateKey()
	toAddress := toPrivKey.Public().Address()
	input := &TxInput{
		PrevTxHash:   initialUTXO.TxHash,
		PrevOutIndex: initialUTXO.Index,
		PublicKey:    privKey.Public(),
	}
	output := &TxOutput{
		Value:   500,
		Address: toAddress,
	}
	changeOutput := &TxOutput{
		Value:   499, // 1 for fee
		Address: privKey.Public().Address(),
	}
	validTx := NewTransaction([]*TxInput{input}, []*TxOutput{output, changeOutput})
	if err := validTx.Sign(privKey); err != nil {
		t.Fatalf("Failed to sign valid transaction: %v", err)
	}

	// Test 1: Valid Block
	header1 := &Header{
		Version:    1,
		PrevHash:   bc.Head().Hash(),
		Height:     bc.Head().Height + 1,
		Timestamp:  time.Now().Unix(),
		Difficulty: bc.Head().Difficulty,
	}
	_, expectedEMABlockTime := bc.CalculateNextDifficulty(bc.Head(), header1.Timestamp)
	header1.EMABlockTime = expectedEMABlockTime
	block1 := NewBlock(header1, []*Transaction{validTx})
	mTree, _ := NewMerkleTree(block1.Transactions)
	block1.Header.MerkleRoot = mTree.RootNode.Data
	pow := NewProofOfWork(block1)
	nonce, _, _ := pow.Run()
	block1.Header.Nonce = nonce

	if err := bc.ValidateBlock(block1); err != nil {
		t.Errorf("Test 1 (Valid Block): ValidateBlock failed: %v", err)
	}

	// Test 2: Invalid PoW (tamper with nonce)
	invalidPoWBlock := NewBlock(header1, []*Transaction{validTx})
	invalidPoWBlock.Header.MerkleRoot = mTree.RootNode.Data
	invalidPoWBlock.Header.Nonce = nonce + 1 // Tamper nonce
	if err := bc.ValidateBlock(invalidPoWBlock); err == nil {
		t.Error("Test 2 (Invalid PoW): ValidateBlock succeeded for invalid PoW")
	}

	// Test 3: Invalid Merkle Root (tamper with transaction)
	invalidMerkleTx := NewTransaction([]*TxInput{input}, []*TxOutput{{Value: 1, Address: toAddress}}) // Different transaction
	invalidMerkleBlock := NewBlock(header1, []*Transaction{invalidMerkleTx})
	// Don't update MerkleRoot, so it will be wrong
	invalidMerkleBlock.Header.Nonce = nonce // Use valid nonce
	if err := bc.ValidateBlock(invalidMerkleBlock); err == nil {
		t.Error("Test 3 (Invalid Merkle Root): ValidateBlock succeeded for invalid Merkle Root")
	}

	// Test 4: Invalid Previous Hash (linkage)
	invalidPrevHashHeader := &Header{
		Version:    1,
		PrevHash:   crypto.Hash{1, 2, 3}, // Tamper PrevHash
		Height:     bc.Head().Height + 1,
		Timestamp:  time.Now().Unix(),
		Difficulty: bc.Head().Difficulty,
	}
	_, expectedEMABlockTime = bc.CalculateNextDifficulty(bc.Head(), invalidPrevHashHeader.Timestamp)
	invalidPrevHashHeader.EMABlockTime = expectedEMABlockTime
	invalidPrevHashBlock := NewBlock(invalidPrevHashHeader, []*Transaction{validTx})
	mTree2, _ := NewMerkleTree(invalidPrevHashBlock.Transactions)
	invalidPrevHashBlock.Header.MerkleRoot = mTree2.RootNode.Data
	pow2 := NewProofOfWork(invalidPrevHashBlock)
	nonce2, _, _ := pow2.Run()
	invalidPrevHashBlock.Header.Nonce = nonce2

	if err := bc.ValidateBlock(invalidPrevHashBlock); err == nil {
		t.Error("Test 4 (Invalid Prev Hash): ValidateBlock succeeded for invalid Prev Hash")
	}

	// Test 5: Invalid Difficulty (tamper with difficulty)
	invalidDifficultyHeader := &Header{
		Version:    1,
		PrevHash:   bc.Head().Hash(),
		Height:     bc.Head().Height + 1,
		Timestamp:  time.Now().Unix(),
		Difficulty: bc.Head().Difficulty + 1, // Tamper Difficulty
	}
	_, expectedEMABlockTime = bc.CalculateNextDifficulty(bc.Head(), invalidDifficultyHeader.Timestamp)
	invalidDifficultyHeader.EMABlockTime = expectedEMABlockTime // EMABlockTime is correct
	invalidDifficultyBlock := NewBlock(invalidDifficultyHeader, []*Transaction{validTx})
	mTree3, _ := NewMerkleTree(invalidDifficultyBlock.Transactions)
	invalidDifficultyBlock.Header.MerkleRoot = mTree3.RootNode.Data
	pow3 := NewProofOfWork(invalidDifficultyBlock)
	nonce3, _, _ := pow3.Run()
	invalidDifficultyBlock.Header.Nonce = nonce3

	if err := bc.ValidateBlock(invalidDifficultyBlock); err == nil {
		t.Error("Test 5 (Invalid Difficulty): ValidateBlock succeeded for invalid Difficulty")
	}

	// Test 6: Invalid EMABlockTime (tamper with EMABlockTime)
	invalidEMABlockTimeHeader := &Header{
		Version:    1,
		PrevHash:   bc.Head().Hash(),
		Height:     bc.Head().Height + 1,
		Timestamp:  time.Now().Unix(),
		Difficulty: bc.Head().Difficulty,
	}
	// Tamper EMABlockTime
	invalidEMABlockTimeHeader.EMABlockTime = expectedEMABlockTime + 1000 
	invalidEMABlockTimeBlock := NewBlock(invalidEMABlockTimeHeader, []*Transaction{validTx})
	mTree4, _ := NewMerkleTree(invalidEMABlockTimeBlock.Transactions)
	invalidEMABlockTimeBlock.Header.MerkleRoot = mTree4.RootNode.Data
	pow4 := NewProofOfWork(invalidEMABlockTimeBlock)
	nonce4, _, _ := pow4.Run()
	invalidEMABlockTimeBlock.Header.Nonce = nonce4

	if err := bc.ValidateBlock(invalidEMABlockTimeBlock); err == nil {
		t.Error("Test 6 (Invalid EMABlockTime): ValidateBlock succeeded for invalid EMABlockTime")
	}
}