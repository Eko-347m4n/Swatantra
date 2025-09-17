package storage

import (
	"encoding/hex"
	"log"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// Iterator is an interface for iterating over key-value pairs.
type Iterator interface {
	Next() bool
	Key() []byte
	Value() []byte
	Close()
}

// Store adalah interface untuk penyimpanan key-value.
type Store interface {
	Put([]byte, []byte) error
	Get([]byte) ([]byte, error)
	Delete([]byte) error
	Has([]byte) (bool, error)
	Close() error
	NewIterator(prefix []byte) Iterator
}

// LevelDBStore adalah implementasi dari Store menggunakan LevelDB.
type LevelDBStore struct {
	db *leveldb.DB
}

// NewLevelDBStore membuat dan mengembalikan instance baru dari LevelDBStore.
func NewLevelDBStore(path string) (*LevelDBStore, error) {
	opts := &opt.Options{
		ErrorIfMissing: false, // Jika database tidak ada, buat baru
	}
	db, err := leveldb.OpenFile(path, opts)
	if err != nil {
		return nil, err
	}

	return &LevelDBStore{
		db: db,
	}, nil
}

// Put menyimpan pasangan key-value.
func (s *LevelDBStore) Put(key, value []byte) error {
	log.Printf("STORAGE: PUT key=%s", hex.EncodeToString(key))
	return s.db.Put(key, value, nil)
}

// Get mengambil nilai berdasarkan key.
func (s *LevelDBStore) Get(key []byte) ([]byte, error) {
	log.Printf("STORAGE: GET key=%s", hex.EncodeToString(key))
	val, err := s.db.Get(key, nil)
	if err != nil {
		log.Printf("STORAGE: GET key=%s, err: %v", hex.EncodeToString(key), err)
	}
	return val, err
}

// Has memeriksa apakah sebuah key ada di dalam database.
func (s *LevelDBStore) Has(key []byte) (bool, error) {
	return s.db.Has(key, nil)
}

// Delete menghapus pasangan key-value.
func (s *LevelDBStore) Delete(key []byte) error {
	return s.db.Delete(key, nil)
}

// Close menutup koneksi database.
func (s *LevelDBStore) Close() error {
	return s.db.Close()
}

// levelDBIterator is an implementation of Iterator for LevelDB.
type levelDBIterator struct {
	it iterator.Iterator
}

func (i *levelDBIterator) Next() bool {
	return i.it.Next()
}

func (i *levelDBIterator) Key() []byte {
	return i.it.Key()
}

func (i *levelDBIterator) Value() []byte {
	return i.it.Value()
}

func (i *levelDBIterator) Close() {
	i.it.Release()
}

// NewIterator creates a new iterator over a key prefix.
func (s *LevelDBStore) NewIterator(prefix []byte) Iterator {
	return &levelDBIterator{
		it: s.db.NewIterator(util.BytesPrefix(prefix), nil),
	}
}
