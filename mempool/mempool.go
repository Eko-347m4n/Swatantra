package mempool

import (
	"errors"
	"sync"

	"swatantra/core"
	"swatantra/crypto"
)

var (
	ErrTxInMempool = errors.New("transaction already in mempool")
)

// Mempool adalah cache untuk transaksi yang belum dikonfirmasi.
type Mempool struct {
	lock       sync.RWMutex
	pool       map[crypto.Hash]*core.Transaction
	blockchain *core.Blockchain
	maxSize    int
}

// NewMempool membuat instance baru dari Mempool.
func NewMempool(bc *core.Blockchain, maxSize int) *Mempool {
	return &Mempool{
		pool:       make(map[crypto.Hash]*core.Transaction),
		blockchain: bc,
		maxSize:    maxSize,
	}
}

// Add menambahkan transaksi ke mempool setelah validasi.
func (mp *Mempool) Add(tx *core.Transaction) error {
	mp.lock.Lock()
	defer mp.lock.Unlock()

	// Cek kapasitas
	if len(mp.pool) >= mp.maxSize {
		return errors.New("mempool is full")
	}

	txHash, err := tx.Hash()
	if err != nil {
		return err
	}

	// Cek apakah sudah ada
	if _, ok := mp.pool[txHash]; ok {
		return ErrTxInMempool
	}

	// Validasi transaksi terhadap state blockchain saat ini
	valid, err := mp.blockchain.ValidateTransaction(tx)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("invalid transaction")
	}

	mp.pool[txHash] = tx
	return nil
}

// GetTransactions mengembalikan sejumlah transaksi dari pool.
func (mp *Mempool) GetTransactions(max int) []*core.Transaction {
	mp.lock.RLock()
	defer mp.lock.RUnlock()

	txs := make([]*core.Transaction, 0, len(mp.pool))
	for _, tx := range mp.pool {
		if len(txs) >= max {
			break
		}
		txs = append(txs, tx)
	}
	return txs
}

// Remove menghapus transaksi dari pool.
func (mp *Mempool) Remove(txHash crypto.Hash) {
	mp.lock.Lock()
	defer mp.lock.Unlock()

	delete(mp.pool, txHash)
}

// Clear menghapus semua transaksi dari pool.
func (mp *Mempool) Clear() {
	mp.lock.Lock()
	defer mp.lock.Unlock()

	mp.pool = make(map[crypto.Hash]*core.Transaction)
}

// Contains memeriksa apakah transaksi dengan hash tertentu ada di mempool.
func (mp *Mempool) Contains(hash crypto.Hash) bool {
	mp.lock.RLock()
	defer mp.lock.RUnlock()

	_, ok := mp.pool[hash]
	return ok
}

// Get mengembalikan transaksi dari mempool berdasarkan hash.
func (mp *Mempool) Get(hash crypto.Hash) (*core.Transaction, error) {
	mp.lock.RLock()
	defer mp.lock.RUnlock()

	tx, ok := mp.pool[hash]
	if !ok {
		return nil, errors.New("transaction not found in mempool")
	}
	return tx, nil
}
