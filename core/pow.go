package core

import (
	"math/big"

	"swatantra/crypto"
)

const (
	// EMAAlpha adalah faktor penghalusan untuk EMA. (2 / (N + 1)), N=1 -> Alpha=1 (tidak ada smoothing)
	// Kita akan gunakan pendekatan yang lebih sederhana untuk penyesuaian per-block.
	DifficultyAdjustmentAlpha = 0.5
)

// ProofOfWork merepresentasikan proses mining dan validasi PoW.
type ProofOfWork struct {
	block  *Block
	target *big.Int
}

// NewProofOfWork membuat instance baru dari ProofOfWork.
func NewProofOfWork(b *Block) *ProofOfWork {
	target := big.NewInt(1)
	// Geser bit ke kiri untuk menentukan target. Semakin besar difficulty, semakin kecil target.
	// 256 adalah panjang hash dalam bit (SHA-256).
	target.Lsh(target, uint(256-b.Header.Difficulty))

	return &ProofOfWork{
		block:  b,
		target: target,
	}
}

// Run menjalankan loop mining untuk menemukan nonce yang valid.
func (pow *ProofOfWork) Run() (uint64, crypto.Hash, error) {
	var hashInt big.Int
	var hash crypto.Hash
	nonce := uint64(0)

	for {
		pow.block.Header.Nonce = nonce
		headerBytes, err := pow.block.Header.EncodeForHashing()
		if err != nil {
			return 0, crypto.Hash{}, err
		}

		hash = crypto.Keccak256(headerBytes)
		hashInt.SetBytes(hash[:])

		if hashInt.Cmp(pow.target) == -1 {
			// Ditemukan hash yang valid (lebih kecil dari target)
			break
		}
		nonce++
	}

	return nonce, hash, nil
}

// Validate memvalidasi apakah PoW dari sebuah block benar.
func (pow *ProofOfWork) Validate() (bool, error) {
	var hashInt big.Int
	headerBytes, err := pow.block.Header.EncodeForHashing()
	if err != nil {
		return false, err
	}

	hash := crypto.Keccak256(headerBytes)
	hashInt.SetBytes(hash[:])

	return hashInt.Cmp(pow.target) == -1, nil
}

// Work mengembalikan jumlah "work" yang direpresentasikan oleh PoW.
// Ini adalah 2^256 / (target + 1)
func (pow *ProofOfWork) Work() *big.Int {
	// numerator = 2^256
	numerator := new(big.Int)
	numerator.Lsh(big.NewInt(1), 256)

	// denominator = target + 1
	denominator := new(big.Int).Add(pow.target, big.NewInt(1))

	return new(big.Int).Div(numerator, denominator)
}
