package p2p

import (
	"swatantra/core"
	"swatantra/crypto"
)

// MessageType adalah enum untuk tipe pesan jaringan.
type MessageType byte

const (
	MessageTypeTx        MessageType = 0x1
	MessageTypeBlock     MessageType = 0x2
	MessageTypeGetBlocks MessageType = 0x3
	MessageTypeInv       MessageType = 0x4
	MessageTypeGetData   MessageType = 0x5
	MessageTypeHandshake MessageType = 0x6
)

// Message merepresentasikan pesan yang dikirim antar peer.
type Message struct {
	Type    MessageType
	Payload []byte
}

// HandshakePayload adalah payload untuk pesan handshake.
type HandshakePayload struct {
	Version    string
	Height     uint32
	HeadHash   crypto.Hash
	ListenAddr string
}

// TxPayload adalah payload untuk pesan transaksi.
type TxPayload struct {
	Tx *core.Transaction
}

// BlockPayload adalah payload untuk pesan block.
type BlockPayload struct {
	Block *core.Block
}

// GetBlocksPayload adalah payload untuk meminta block.
type GetBlocksPayload struct {
	From crypto.Hash // Hash dari block mana permintaan dimulai
	To   crypto.Hash // Hash dari block mana permintaan berakhir (opsional)
}

// InvPayload adalah payload untuk pesan inventory.
// Ini digunakan untuk memberitahu peer tentang data baru yang kita miliki.
type InvPayload struct {
	Type    byte          // 'b' untuk block, 't' untuk transaksi
	Hashes  []crypto.Hash
}

// GetDataPayload adalah payload untuk meminta data spesifik (block atau tx).
type GetDataPayload struct {
	Type byte        // 'b' untuk block, 't' untuk transaksi
	Hash crypto.Hash // Hash dari data yang diminta
}
