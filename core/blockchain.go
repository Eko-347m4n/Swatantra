package core

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"time"

	"swatantra/crypto"
	"swatantra/storage"
)

var (
	ErrBlockNotFound = errors.New("block not found")
)

// Blockchain adalah komponen utama yang mengelola state, termasuk block dan UTXO set.
type Blockchain struct {
	store      storage.Store
	blockStore *BlockStore
	headers    map[crypto.Hash]*Header // Menyimpan semua header untuk melacak fork
	head       *Header                 // Header dari block terakhir di main chain
}

var headKey = []byte("head")

// Head mengembalikan header dari block terakhir di main chain.
func (bc *Blockchain) Head() *Header {
	return bc.head
}

// NewBlockchain membuat instance baru dari Blockchain.
func NewBlockchain(s storage.Store, initialDifficulty uint32) (*Blockchain, error) {
	bs := NewBlockStore(s)
	bc := &Blockchain{
		store:      s,
		blockStore: bs,
		headers:    make(map[crypto.Hash]*Header),
	}

	headHashBytes, err := s.Get(headKey)
	if err != nil {
		// Asumsikan error berarti tidak ada head, jadi kita buat genesis block
		fmt.Println("No head found, creating genesis block...")
		genesis := CreateGenesisBlock(crypto.Address{}, 1000, initialDifficulty) // Alamat dan supply awal
		
		// Add genesis block directly without full validation
		blockHash, _ := genesis.Hash()
		bc.headers[blockHash] = genesis.Header
		bc.head = genesis.Header
		if err := bc.blockStore.Put(genesis); err != nil {
			return nil, err
		}
		if err := bc.updateUTXOSet(genesis); err != nil {
			return nil, err
		}
		if err := s.Put(headKey, blockHash[:]); err != nil {
			return nil, err
		}
	} else {
		// Load head dari DB
		var headHash crypto.Hash
		copy(headHash[:], headHashBytes)
		headHeader, err := bs.GetHeader(headHash)
		if err != nil {
			return nil, err
		}
		bc.head = headHeader
		// TODO: Load all headers into bc.headers
	}

	return bc, nil
}

const (
	// TargetBlockTime adalah waktu target antar block.
	TargetBlockTime = 15 * time.Second
	// DifficultyAdjustmentInterval adalah interval dalam block untuk menyesuaikan difficulty.
	// Untuk EMA, kita sesuaikan di setiap block.
	DifficultyAdjustmentInterval = 1
	// EMAAlphaNumerator dan Denominator untuk faktor penghalusan EMA. (2 / (N + 1)).
	// N=20 -> alpha approx 0.095. Kita gunakan 95/1000.
	emaAlphaNumerator   = 95
	emaAlphaDenominator = 1000
)

// CalculateNextDifficulty menghitung difficulty dan EMA block time berikutnya.
func (bc *Blockchain) CalculateNextDifficulty(parentHeader *Header, newTimestamp int64) (uint32, int64) {
	// Untuk genesis block, difficulty sudah di-hardcode
	if parentHeader.Height == 0 {
		return parentHeader.Difficulty, parentHeader.EMABlockTime
	}

	// Waktu block aktual
	actualBlockTime := newTimestamp - parentHeader.Timestamp

	// EMA block time sebelumnya
	prevEMABlockTime := parentHeader.EMABlockTime

	// Hitung EMA baru
	// EMA = (alpha * current_value) + ((1 - alpha) * prev_ema)
	newEMABlockTime := (emaAlphaNumerator*actualBlockTime + (emaAlphaDenominator-emaAlphaNumerator)*prevEMABlockTime) / emaAlphaDenominator

	var newDifficulty uint32
	// Batas atas dan bawah untuk EMA agar tidak terjadi perubahan ekstrem
	lowerBound := int64(TargetBlockTime) - (int64(TargetBlockTime) / 4) // 75%
	upperBound := int64(TargetBlockTime) + (int64(TargetBlockTime) / 2) // 150%

	if newEMABlockTime < lowerBound {
		// Terlalu cepat, naikan difficulty
		newDifficulty = parentHeader.Difficulty + 1
	} else if newEMABlockTime > upperBound {
		// Terlalu lambat, kurangi difficulty (minimal 1)
		if parentHeader.Difficulty > 1 {
			newDifficulty = parentHeader.Difficulty - 1
		} else {
			newDifficulty = 1
		}
	} else {
		// Dalam rentang target, tidak ada perubahan
		newDifficulty = parentHeader.Difficulty
	}

	return newDifficulty, newEMABlockTime
}

// AddBlock menambahkan block baru ke blockchain, menangani fork.
func (bc *Blockchain) AddBlock(b *Block) error {
	blockHash, _ := b.Hash()
	// Cek apakah block sudah ada
	if _, ok := bc.headers[blockHash]; ok {
		return nil // Anggap block sudah diproses
	}

	// Validasi block SEBELUM menambahkannya ke mana pun
	if err := bc.ValidateBlock(b); err != nil {
		return err
	}

	// Ambil header parent untuk menghitung cumulative work.
	// ValidateBlock seharusnya sudah memastikan header ini ada di bc.headers atau di store.
	prevHeader := bc.headers[b.Header.PrevHash]

	// Hitung cumulative work
	work := NewProofOfWork(b).Work()
	b.Header.CumulativeWork = new(big.Int).Add(prevHeader.CumulativeWork, work)

	// Simpan block dan header
	if err := bc.blockStore.Put(b); err != nil {
		return err
	}
	bc.headers[blockHash] = b.Header

	// Cek apakah ini adalah perpanjangan rantai biasa (bukan fork)
	currentHeadHash := bc.head.Hash()
	if b.Header.PrevHash == currentHeadHash {
		if err := bc.updateUTXOSet(b); err != nil {
			return err // Error kritis
		}
		// Perbarui head
		bc.head = b.Header
		return bc.store.Put(headKey, blockHash[:])
	}

	// Jika bukan perpanjangan biasa, ini adalah fork.
	// Cek apakah fork ini memiliki cumulative work yang lebih besar.
	if b.Header.CumulativeWork.Cmp(bc.head.CumulativeWork) > 0 {
		// Hanya panggil reorg jika kita berada di fork yang lebih baik
		return bc.reorganizeChain(b)
	}

	// Jika kita menerima block dari fork yang lebih lemah, abaikan (tapi tetap simpan).
	fmt.Printf("Received a fork block %s, but our current chain has more work.\n", blockHash.ToHex())
	return nil
}


// reorganizeChain mengatur ulang chain untuk menjadikan block baru sebagai head.
func (bc *Blockchain) reorganizeChain(newHeadBlock *Block) error {
	fmt.Println("Reorganizing chain...")
	
	newHeadHash, _ := newHeadBlock.Hash()
	var oldHeadHash crypto.Hash
	if bc.head != nil {
		oldHeadHash = bc.head.Hash()
	}

	// 1. Temukan common ancestor
	ancestorHash, err := bc.findCommonAncestor(oldHeadHash, newHeadHash)
	if err != nil {
		return fmt.Errorf("could not find common ancestor: %v", err)
	}
	fmt.Printf("Common ancestor found: %s\n", ancestorHash.ToHex())

	// 2. Buat daftar block untuk di-rollback dan di-apply
	blocksToRollback, err := bc.getChainPath(oldHeadHash, ancestorHash)
	if err != nil {
		return fmt.Errorf("could not get path to rollback: %v", err)
	}
	blocksToApply, err := bc.getChainPath(newHeadHash, ancestorHash)
	if err != nil {
		return fmt.Errorf("could not get path to apply: %v", err)
	}

	// 3. Rollback blocks (dalam urutan terbalik)
	for i := 0; i < len(blocksToRollback); i++ {
		blockHash := blocksToRollback[i]
		block, err := bc.blockStore.Get(blockHash)
		if err != nil {
			return err
		}
		fmt.Printf("Rolling back block %s (height %d)\n", blockHash.ToHex(), block.Header.Height)
		if err := bc.rollbackUTXOSet(block); err != nil {
			return err
		}
	}

	// 4. Apply blocks (dalam urutan terbalik karena getChainPath mengembalikan dari head)
	for i := len(blocksToApply) - 1; i >= 0; i-- {
		blockHash := blocksToApply[i]
		block, err := bc.blockStore.Get(blockHash)
		if err != nil {
			return err
		}
		fmt.Printf("Applying block %s (height %d)\n", blockHash.ToHex(), block.Header.Height)
		if err := bc.updateUTXOSet(block); err != nil {
			return err
		}
	}

	// 5. Update head
	bc.head = newHeadBlock.Header
	if err := bc.store.Put(headKey, newHeadHash[:]); err != nil { // newHeadHash is Block.Hash()
		return err
	}

	fmt.Println("Reorganization complete.")
	return nil
}

// findCommonAncestor menemukan nenek moyang bersama dari dua block.
func (bc *Blockchain) findCommonAncestor(hashA, hashB crypto.Hash) (crypto.Hash, error) {
	if hashA.IsZero() || hashB.IsZero() {
		return crypto.Hash{}, nil // Genesis is the ancestor
	}

	pathA, err := bc.getChainPath(hashA, crypto.Hash{})
	if err != nil {
		return crypto.Hash{}, err
	}
	pathB, err := bc.getChainPath(hashB, crypto.Hash{})
	if err != nil {
		return crypto.Hash{}, err
	}

	setA := make(map[crypto.Hash]bool)
	for _, hash := range pathA {
		setA[hash] = true
	}

	for _, hash := range pathB {
		if setA[hash] {
			return hash, nil
		}
	}

	return crypto.Hash{}, errors.New("no common ancestor found (should not happen if both have genesis)")
}

// getChainPath mengembalikan path dari startHash ke endHash (tidak termasuk endHash).
func (bc *Blockchain) getChainPath(startHash, endHash crypto.Hash) ([]crypto.Hash, error) {
	path := []crypto.Hash{}
	if startHash.IsZero() {
		return path, nil
	}
	currentHash := startHash
	for currentHash != endHash && !currentHash.IsZero() {
		path = append(path, currentHash)
		header, ok := bc.headers[currentHash]
		if !ok {
			return nil, fmt.Errorf("header not found for hash %s", currentHash.ToHex())
		}
		if header.Height == 0 {
			break
		}
		currentHash = header.PrevHash
	}
	return path, nil
}

// rollbackUTXOSet membatalkan perubahan UTXO dari sebuah block.
func (bc *Blockchain) rollbackUTXOSet(b *Block) error {
	// 1. Ambil data undo
	blockHash, _ := b.Hash()
	undoKey := getUndoKey(blockHash)
	undoData, err := bc.store.Get(undoKey)
	if err != nil {
		return fmt.Errorf("could not find undo data for block %s", blockHash.ToHex())
	}
	
	var undoBlock BlockUndo
	if err := undoBlock.Decode(undoData); err != nil {
		return err
	}

	// 2. Hapus output yang dibuat oleh block ini
	for _, tx := range b.Transactions {
		txHash, _ := tx.Hash()
		for i := range tx.Outputs {
			key := getUTXOKey(txHash, uint32(i))
			if err := bc.store.Delete(key); err != nil {
				return err
			}
		}
	}

	// 3. Kembalikan output yang dihabiskan oleh block ini
	for _, spentUTXO := range undoBlock.SpentUTXOs {
		key := getUTXOKey(spentUTXO.TxHash, spentUTXO.Index)
		encoded, err := spentUTXO.Output.Encode()
		if err != nil {
			return err
		}
		if err := bc.store.Put(key, encoded); err != nil {
			return err
		}
	}
	
	// Hapus data undo setelah selesai
	return bc.store.Delete(undoKey)
}

// updateUTXOSet memperbarui UTXO set berdasarkan block baru dan menyimpan data undo.
func (bc *Blockchain) updateUTXOSet(b *Block) error {
	undoBlock := &BlockUndo{SpentUTXOs: []*SpentUTXO{}}

	// Hapus input dari UTXO set dan kumpulkan untuk data undo
	for _, tx := range b.Transactions {
		if !tx.IsCoinbase() {
			for _, input := range tx.Inputs {
				spentOutput, err := bc.GetUTXO(input.PrevTxHash, input.PrevOutIndex)
				if err != nil {
					// Ini seharusnya tidak terjadi jika block sudah divalidasi
					return fmt.Errorf("could not find UTXO for input %s:%d", input.PrevTxHash.ToHex(), input.PrevOutIndex)
				}
				spentUTXO := &SpentUTXO{
						TxHash: input.PrevTxHash,
						Index:  input.PrevOutIndex,
						Output: spentOutput,
					}
				undoBlock.SpentUTXOs = append(undoBlock.SpentUTXOs, spentUTXO)

				key := getUTXOKey(input.PrevTxHash, input.PrevOutIndex)
				if err := bc.store.Delete(key); err != nil {
					return err
				}
			}
		}
	}

	// Tambahkan output baru ke UTXO set
	for _, tx := range b.Transactions {
		txHash, err := tx.Hash()
		if err != nil {
			return err
		}
		for i, output := range tx.Outputs {
			key := getUTXOKey(txHash, uint32(i))
			encoded, err := output.Encode()
			if err != nil {
				return err
			}
			if err := bc.store.Put(key, encoded); err != nil {
				return err
			}
		}
	}

	// Simpan data undo
	blockHash, _ := b.Hash()
	undoKey := getUndoKey(blockHash)
	undoData, err := undoBlock.Encode()
	if err != nil {
		return err
	}
	return bc.store.Put(undoKey, undoData)
}


func (bc *Blockchain) ValidateBlock(b *Block) error {
	if b.Header.Height > 0 {
		prevHeader, ok := bc.headers[b.Header.PrevHash]
		if !ok {
			fmt.Printf("ValidateBlock: Parent header %s not in memory. Trying blockStore.\n", b.Header.PrevHash.ToHex())
			// Try to get parent from blockStore if not in memory
			var err error
			prevHeader, err = bc.blockStore.GetHeader(b.Header.PrevHash)
			if err != nil {
				return fmt.Errorf("parent block %s not found for validation: %v", b.Header.PrevHash.ToHex(), err)
			}
			fmt.Printf("ValidateBlock: Parent header %s found in blockStore.\n", b.Header.PrevHash.ToHex())
			// Add to in-memory headers for future quick access
			bc.headers[b.Header.PrevHash] = prevHeader
		}
		if b.Header.Height != prevHeader.Height+1 {
			return errors.New("invalid height")
		}
		
		// Validasi difficulty
		expectedDifficulty, expectedEMABlockTime := bc.CalculateNextDifficulty(prevHeader, b.Header.Timestamp)
		if b.Header.Difficulty != expectedDifficulty {
			return fmt.Errorf("invalid difficulty: got %d, expected %d", b.Header.Difficulty, expectedDifficulty)
		}
		if b.Header.EMABlockTime != expectedEMABlockTime {
			return fmt.Errorf("invalid EMABlockTime: got %d, expected %d", b.Header.EMABlockTime, expectedEMABlockTime)
		}

	} else { // This is the genesis block
		if !b.Header.PrevHash.IsZero() {
			return errors.New("genesis block must have zero prevhash")
		}
	}

	pow := NewProofOfWork(b)
	valid, err := pow.Validate()
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("invalid proof of work")
	}

	mTree, err := NewMerkleTree(b.Transactions)
	if err != nil {
		return err
	}
	if mTree.RootNode.Data != b.Header.MerkleRoot {
		return errors.New("invalid merkle root")
	}

	for _, tx := range b.Transactions {
		if !tx.IsCoinbase() {
			valid, err := bc.ValidateTransaction(tx)
			if err != nil {
				return err
			}
			if !valid {
				return errors.New("invalid transaction in block")
			}
		}
	}

	return nil
}

func (bc *Blockchain) HasUTXO(hash crypto.Hash, index uint32) (bool, error) {
	key := getUTXOKey(hash, index)
	return bc.store.Has(key)
}

// GetBlockByHash mengambil block dari database berdasarkan hash-nya.
func (bc *Blockchain) GetBlockByHash(hash crypto.Hash) (*Block, error) {
	return bc.blockStore.Get(hash)
}

// GetBlocksFrom mengembalikan daftar block dari hash yang diberikan hingga head.
func (bc *Blockchain) GetBlocksFrom(fromHash crypto.Hash) ([]*Block, error) {
	blocks := []*Block{}
	currentHash := bc.head.Hash()

	// Iterate backwards from the head until fromHash is found
	for {
		block, err := bc.blockStore.Get(currentHash)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)

		blockHash, err := block.Hash()
		if err != nil {
			return nil, err
		}
		if blockHash == fromHash {
			break // Found the starting block
		}

		if block.Header.Height == 0 {
			return nil, errors.New("fromHash not found in chain") // Reached genesis without finding fromHash
		}
		currentHash = block.Header.PrevHash
	}

	// Reverse the list to get blocks in forward order
	for i, j := 0, len(blocks)-1; i < j; i, j = i+1, j-1 {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	}

	return blocks, nil
}

func (bc *Blockchain) ValidateTransaction(tx *Transaction) (bool, error) {
	if tx.IsCoinbase() {
		return true, nil
	}
	for _, input := range tx.Inputs {
		ok, err := bc.HasUTXO(input.PrevTxHash, input.PrevOutIndex)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, errors.New("input not found in UTXO set")
		}
	}

	return tx.Verify()
}

var (
	utxoKeyPrefix = []byte("u") // 'u' for UTXO
	undoKeyPrefix = []byte("z") // 'z' for undo
)

func getUTXOKey(hash crypto.Hash, index uint32) []byte {
	key := make([]byte, len(utxoKeyPrefix)+len(crypto.Hash{})+4)
	copy(key, utxoKeyPrefix)
	copy(key[len(utxoKeyPrefix):], hash[:])
	binary.BigEndian.PutUint32(key[len(utxoKeyPrefix)+len(crypto.Hash{}):], index)
	return key
}

func getUndoKey(hash crypto.Hash) []byte {
	return append(undoKeyPrefix, hash[:]...)
}

// GetUTXO finds and returns a specific output from the UTXO set.
func (bc *Blockchain) GetUTXO(hash crypto.Hash, index uint32) (*TxOutput, error) {
	key := getUTXOKey(hash, index)
	data, err := bc.store.Get(key)
	if err != nil {
		return nil, err
	}

	output := &TxOutput{}
	if err := output.Decode(data); err != nil {
		return nil, err
	}

	return output, nil
}

// FindUTXOs finds all unspent transaction outputs for a given address.
// NOTE: This is an inefficient implementation that iterates the whole DB.
// A real wallet should maintain its own UTXO index.
func (bc *Blockchain) FindUTXOs(address crypto.Address) ([]*SpentUTXO, error) {
	var utxos []*SpentUTXO
	it := bc.store.NewIterator(utxoKeyPrefix)
	defer it.Close()

	for it.Next() {
		key := it.Key()
		val := it.Value()

		output := &TxOutput{}
		if err := output.Decode(val); err != nil {
			// Corrupted data, maybe log it
			continue
		}

		if output.Address == address {
			// We need to parse the tx hash and index from the key
			txHash := crypto.Hash{}
			// key = u<tx_hash><index>
			copy(txHash[:], key[len(utxoKeyPrefix):len(utxoKeyPrefix)+32])
			index := binary.BigEndian.Uint32(key[len(utxoKeyPrefix)+32:])
			
			utxos = append(utxos, &SpentUTXO{
				TxHash: txHash,
				Index:  index,
				Output: output,
			})
		}
	}
	return utxos, nil
}