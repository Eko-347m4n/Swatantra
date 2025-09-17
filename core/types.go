package core

import (
	"bytes"
	"encoding/gob"
	"math/big"

	"swatantra/crypto"
)

// Header merepresentasikan header dari sebuah block.
type Header struct {
	Version      uint32
	PrevHash     crypto.Hash
	Height       uint32
	MerkleRoot   crypto.Hash
	Timestamp    int64
	Difficulty   uint32
	Nonce        uint64
	EMABlockTime int64 // Exponential Moving Average of block time
	CumulativeWork *big.Int
}

// Encode mengubah Header menjadi slice of bytes menggunakan gob.
func (h *Header) Encode() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(h); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode mengubah slice of bytes menjadi Header menggunakan gob.
func (h *Header) Decode(b []byte) error {
	buf := bytes.NewReader(b)
	dec := gob.NewDecoder(buf)
	return dec.Decode(h)
}

// EncodeForHashing meng-encode header untuk keperluan hashing (tanpa CumulativeWork).
func (h *Header) EncodeForHashing() ([]byte, error) {
	hCopy := *h
	hCopy.CumulativeWork = nil // Abaikan cumulative work dari hash

	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(&hCopy); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Hash menghitung hash dari header.
func (h *Header) Hash() crypto.Hash {
	headerBytes, err := h.EncodeForHashing()
	if err != nil {
		// Seharusnya tidak terjadi jika header valid
		panic(err)
	}
	return crypto.Keccak256(headerBytes)
}

// Block merepresentasikan satu block dalam blockchain.
type Block struct {
	*Header
	Transactions []*Transaction

	hash crypto.Hash // Hash dari header, di-cache
}

// NewBlock membuat block baru.
func NewBlock(h *Header, txs []*Transaction) *Block {
	return &Block{
		Header:       h,
		Transactions: txs,
	}
}

// Hash menghitung hash dari header block.
func (b *Block) Hash() (crypto.Hash, error) {
	if !b.hash.IsZero() {
		return b.hash, nil
	}

	b.hash = b.Header.Hash()
	return b.hash, nil
}

// Encode mengubah Block menjadi slice of bytes menggunakan gob.
func (b *Block) Encode() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(b); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode mengubah slice of bytes menjadi Block menggunakan gob.
func (b *Block) Decode(data []byte) error {
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(b)
}

// TxInput merepresentasikan sebuah input dalam transaksi.
type TxInput struct {
	PrevTxHash crypto.Hash // Hash dari transaksi sebelumnya
	PrevOutIndex uint32      // Indeks output di transaksi sebelumnya
	PublicKey  crypto.PublicKey
	Signature  []byte
}

// TxOutput merepresentasikan sebuah output dalam transaksi.
type TxOutput struct {
	Value   uint64
	Address crypto.Address
}

// Encode mengubah TxOutput menjadi slice of bytes.
func (o *TxOutput) Encode() ([]byte, error) {
    buf := new(bytes.Buffer)
    if err := gob.NewEncoder(buf).Encode(o); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// Decode mengubah slice of bytes menjadi TxOutput.
func (o *TxOutput) Decode(b []byte) error {
    buf := bytes.NewReader(b)
    return gob.NewDecoder(buf).Decode(o)
}

// Transaction merepresentasikan sebuah transaksi.
type Transaction struct {
	Inputs  []*TxInput
	Outputs []*TxOutput

	hash crypto.Hash // Hash dari transaksi, di-cache
}

// NewTransaction membuat transaksi baru.
func NewTransaction(inputs []*TxInput, outputs []*TxOutput) *Transaction {
	return &Transaction{
		Inputs:  inputs,
		Outputs: outputs,
	}
}

// Hash menghitung hash dari transaksi.
func (tx *Transaction) Hash() (crypto.Hash, error) {
	if !tx.hash.IsZero() {
		return tx.hash, nil
	}

	// Untuk menghitung hash, kita tidak menyertakan signature dan publickey di input
	txCopy := *tx
	txCopy.Inputs = make([]*TxInput, len(tx.Inputs))
	for i, input := range tx.Inputs {
		txCopy.Inputs[i] = &TxInput{
			PrevTxHash:   input.PrevTxHash,
			PrevOutIndex: input.PrevOutIndex,
		}
	}

	encoded, err := txCopy.EncodeForHashing()
	if err != nil {
		return crypto.Hash{}, err
	}

	tx.hash = crypto.Keccak256(encoded)
	return tx.hash, nil
}

// EncodeForHashing meng-encode transaksi tanpa signature untuk hashing.
func (tx *Transaction) EncodeForHashing() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	if err := enc.Encode(tx); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Sign menandatangani semua input dalam transaksi.
func (tx *Transaction) Sign(privKey crypto.PrivateKey) error {
	hash, err := tx.Hash()
	if err != nil {
		return err
	}

	for _, input := range tx.Inputs {
		sig, err := privKey.Sign(hash[:])
		if err != nil {
			return err
		}
		input.Signature = sig
		input.PublicKey = privKey.Public()
	}
	return nil
}

// Verify memverifikasi semua tanda tangan input dalam transaksi.
func (tx *Transaction) Verify() (bool, error) {
	if tx.IsCoinbase() {
		return true, nil // Coinbase tidak perlu verifikasi signature
	}

	hash, err := tx.Hash()
	if err != nil {
		return false, err
	}

	for _, input := range tx.Inputs {
		if !input.PublicKey.Verify(hash[:], input.Signature) {
			return false, nil
		}
	}
	return true, nil
}

// IsCoinbase memeriksa apakah transaksi ini adalah transaksi coinbase.
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 && tx.Inputs[0].PrevTxHash.IsZero()
}

// BlockUndo menyimpan informasi untuk membatalkan perubahan UTXO dari sebuah block.
type BlockUndo struct {
	SpentUTXOs []*SpentUTXO
}

// SpentUTXO adalah UTXO yang dihabiskan, disimpan untuk keperluan undo.
type SpentUTXO struct {
	TxHash crypto.Hash
	Index  uint32
	Output *TxOutput
}

// Encode mengubah BlockUndo menjadi slice of bytes.
func (u *BlockUndo) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(u); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode mengubah slice of bytes menjadi BlockUndo.
func (u *BlockUndo) Decode(b []byte) error {
	buf := bytes.NewReader(b)
	return gob.NewDecoder(buf).Decode(u)
}
