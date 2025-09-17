package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"

	"crypto/ed25519"
)

// HexToHash converts a hex string to a Hash.
func HexToHash(s string) Hash {
	var h Hash
	decoded, _ := hex.DecodeString(s)
	copy(h[:], decoded)
	return h
}

func TestGeneratePrivateKey(t *testing.T) {
	privKey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	if len(privKey) != ed25519.PrivateKeySize {
		t.Errorf("Expected private key size %d, got %d", ed25519.PrivateKeySize, len(privKey))
	}
	pubKey := privKey.Public()
	if len(pubKey) != ed25519.PublicKeySize {
		t.Errorf("Expected public key size %d, got %d", ed25519.PublicKeySize, len(pubKey))
	}
}

func TestSignAndVerify(t *testing.T) {
	privKey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate private key: %v", err)
	}
	pubKey := privKey.Public()

	data := []byte("test message")
	signature, err := privKey.Sign(data)
	if err != nil {
		t.Fatalf("Failed to sign data: %v", err)
	}

	if !pubKey.Verify(data, signature) {
		t.Error("Signature verification failed for valid signature")
	}

	// Test with tampered data
	tamperedData := []byte("tampered message")
	if pubKey.Verify(tamperedData, signature) {
		t.Error("Signature verification succeeded for tampered data")
	}

	// Test with wrong signature
	wrongSignature := make([]byte, len(signature))
	copy(wrongSignature, signature)
	wrongSignature[0] = ^wrongSignature[0] // Flip a bit
	if pubKey.Verify(data, wrongSignature) {
		t.Error("Signature verification succeeded for wrong signature")
	}

	// Test with wrong public key
	wrongPrivKey, _ := GeneratePrivateKey()
	wrongPubKey := wrongPrivKey.Public()
	if wrongPubKey.Verify(data, signature) {
		t.Error("Signature verification succeeded for wrong public key")
	}
}

func TestKeccak256(t *testing.T) {
	testCases := []struct {
		input    []byte
		expected Hash
	}{
		{
			input:    []byte(""),
			expected: HexToHash("c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"),
		},
		{
			input:    []byte("hello"),
			expected: HexToHash("1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8"),
		},
		{
			input:    []byte("world"),
			expected: HexToHash("8452c9b9140222b08593a26daa782707297be9f7b3e8281d7b4974769f19afd0"),
		},
	}

	for _, tc := range testCases {
		actual := Keccak256(tc.input)
		if !bytes.Equal(actual[:], tc.expected[:]) {
			t.Errorf("Keccak256(%q): expected %x, got %x", tc.input, tc.expected, actual)
		}
	}
}

func TestPublicKeyAddress(t *testing.T) {
	privKey, _ := GeneratePrivateKey()
	pubKey := privKey.Public()
	address := pubKey.Address()

	// Ensure address is of correct length
	if len(address) != AddressLength {
		t.Errorf("Expected address length %d, got %d", AddressLength, len(address))
	}

	// Ensure address is derived consistently
	address2 := pubKey.Address()
	if !bytes.Equal(address[:], address2[:]) {
		t.Error("Address derivation is not consistent")
	}

	// Test with different public key
	privKey2, _ := GeneratePrivateKey()
	pubKey2 := privKey2.Public()
	address3 := pubKey2.Address()

	if bytes.Equal(address[:], address3[:]) {
		t.Error("Different public keys produced same address")
	}
}
