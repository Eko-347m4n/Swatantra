package core

import (
	"fmt" // Added
	"swatantra/crypto"
	"swatantra/storage"
)

// BlockStore bertanggung jawab untuk menyimpan dan mengambil block.
type BlockStore struct {
	store storage.Store
}

// NewBlockStore membuat instance baru dari BlockStore.
func NewBlockStore(s storage.Store) *BlockStore {
	return &BlockStore{
		store: s,
	}
}

// Put menyimpan block ke dalam database.
// Key-nya adalah hash dari block.
func (bs *BlockStore) Put(b *Block) error {
	hash, err := b.Hash()
	if err != nil {
		return err
	}

	encoded, err := b.Encode()
	if err != nil {
		return err
	}
	fmt.Printf("BlockStore: Putting block %s (height %d) to store.\n", hash.ToHex(), b.Header.Height);
	return bs.store.Put(hash[:], encoded)
}

// Get mengambil block dari database berdasarkan hash-nya.
func (bs *BlockStore) Get(hash crypto.Hash) (*Block, error) {
	fmt.Printf("BlockStore: Getting block %s from store.\n", hash.ToHex())
	encoded, err := bs.store.Get(hash[:])
	if err != nil {
		fmt.Printf("BlockStore: Block %s not found in store: %v\n", hash.ToHex(), err)
		return nil, err
	}

	b := new(Block)
	if err := b.Decode(encoded); err != nil {
		return nil, err
	}

	return b, nil
}

// GetHeader mengambil header dari database berdasarkan hash-nya.
func (bs *BlockStore) GetHeader(hash crypto.Hash) (*Header, error) {
	b, err := bs.Get(hash)
	if err != nil {
		return nil, err
	}
	return b.Header, nil
}
