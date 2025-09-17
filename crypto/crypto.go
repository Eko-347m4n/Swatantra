package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/sha3"
)

const (
	AddressLength        = 20
	PrivateKeySeedLength = 32
	SignatureSize        = 64 // ed25519.SignatureSize
)

type Hash [32]byte

func (h Hash) ToHex() string {
	return hex.EncodeToString(h[:])
}

func (h Hash) IsZero() bool {
	return h == (Hash{})
}

type Address [AddressLength]byte

func (a Address) ToHex() string {
	return hex.EncodeToString(a[:])
}

type PublicKey []byte

func (k PublicKey) Address() Address {
	hash := Keccak256(k)
	var addr Address
	copy(addr[:], hash[len(hash)-AddressLength:])
	return addr
}

type PrivateKey []byte

func GeneratePrivateKey() (PrivateKey, error) {
	seed := make([]byte, PrivateKeySeedLength)
	if _, err := io.ReadFull(rand.Reader, seed); err != nil {
		return nil, err
	}
	return PrivateKey(ed25519.NewKeyFromSeed(seed)), nil
}

func (k PrivateKey) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(k), data), nil
}

func (k PrivateKey) Public() PublicKey {
	// The public key is the second half of the 64-byte private key.
	// This is how ed25519 works: the private key is the seed concatenated with the public key.
	return PublicKey(k[32:])
}

func (k PublicKey) Verify(data, signature []byte) bool {
	return ed25519.Verify(ed25519.PublicKey(k), data, signature)
}

func Keccak256(data []byte) Hash {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	var h Hash
	copy(h[:], hash.Sum(nil))
	return h
}