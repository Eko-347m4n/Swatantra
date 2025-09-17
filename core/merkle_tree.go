package core

import (
	"swatantra/crypto"
)

// MerkleTree merepresentasikan sebuah Merkle tree.
type MerkleTree struct {
	RootNode *MerkleNode
}

// MerkleNode merepresentasikan sebuah node dalam Merkle tree.
type MerkleNode struct {
	Left  *MerkleNode
	Right *MerkleNode
	Data  crypto.Hash
}

// NewMerkleNode membuat node baru.
func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	if left == nil && right == nil {
		// Leaf node
		hash := crypto.Keccak256(data)
		node.Data = hash
	} else {
		// Internal node
		prevHashes := append(left.Data[:], right.Data[:]...)
		hash := crypto.Keccak256(prevHashes)
		node.Data = hash
	}

	node.Left = left
	node.Right = right

	return &node
}

// NewMerkleTree membuat Merkle tree dari daftar transaksi.
func NewMerkleTree(transactions []*Transaction) (*MerkleTree, error) {
	var nodes []MerkleNode

	if len(transactions) == 0 {
		return nil, nil // Atau kembalikan hash kosong
	}

	// Buat leaf nodes dari setiap transaksi
	for _, tx := range transactions {
		hash, err := tx.Hash()
		if err != nil {
			return nil, err
		}
		node := NewMerkleNode(nil, nil, hash[:])
		nodes = append(nodes, *node)
	}

	// Bangun tree dari bawah ke atas
	for len(nodes) > 1 {
		// Jika jumlah node ganjil, duplikat yang terakhir
		if len(nodes)%2 != 0 {
			nodes = append(nodes, nodes[len(nodes)-1])
		}

		var newLevel []MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(&nodes[i], &nodes[i+1], nil)
			newLevel = append(newLevel, *node)
		}
		nodes = newLevel
	}

	tree := MerkleTree{&nodes[0]}
	return &tree, nil
}
