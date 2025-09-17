package p2p

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"swatantra/core"
	"swatantra/mempool"
)

// RateLimiter implements a simple token bucket rate limiter.
type RateLimiter struct {
	rate         int64 // tokens per second
	bucketSize   int64
	tokens       int64
	lastRefill   time.Time
	lock         sync.Mutex
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(rate, bucketSize int64) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		bucketSize: bucketSize,
		tokens:     bucketSize,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed. It consumes one token if it is.
func (rl *RateLimiter) Allow() bool {
	rl.lock.Lock()
	defer rl.lock.Unlock()

	// Refill tokens
	now := time.Now()
	duration := now.Sub(rl.lastRefill)
	tokensToAdd := (duration.Nanoseconds() * rl.rate) / 1e9
	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.bucketSize {
			rl.tokens = rl.bucketSize
		}
		rl.lastRefill = now
	}

	// Check if there are enough tokens
	if rl.tokens >= 1 {
		rl.tokens--
		return true
	}

	return false
}

// Peer merepresentasikan node lain yang terhubung.
type Peer struct {
	conn    net.Conn
	encoder *gob.Encoder
	decoder *gob.Decoder
	limiter *RateLimiter
}

func NewPeer(conn net.Conn) *Peer {
	return &Peer{
		conn:    conn,
		encoder: gob.NewEncoder(conn),
		decoder: gob.NewDecoder(conn),
		limiter: NewRateLimiter(10, 100), // 10 msg/sec, burst of 100
	}
}

// Send mengirim pesan ke peer.
func (p *Peer) Send(msg *Message) error {
	return p.encoder.Encode(msg)
}

const BlacklistDuration = 24 * time.Hour // Durasi peer akan berada di daftar hitam

// Server adalah server P2P yang mengelola koneksi peer.
type Server struct {
	listenAddr string
	listener   net.Listener
	peers      map[net.Addr]*Peer
	lock       sync.RWMutex
	blacklist  map[string]time.Time

	msgCh      chan *RPC
	blockchain *core.Blockchain
	mempool    *mempool.Mempool
}

// RPC merepresentasikan remote procedure call yang diterima dari peer.
type RPC struct {
	From    net.Addr
	Payload []byte
	Type    MessageType
}

// NewServer membuat instance baru dari Server.
func NewServer(listenAddr string, bc *core.Blockchain, mp *mempool.Mempool) *Server {
	return &Server{
		listenAddr: listenAddr,
		peers:      make(map[net.Addr]*Peer),
		blacklist:  make(map[string]time.Time),
		msgCh:      make(chan *RPC, 128),
		blockchain: bc,
		mempool:    mp,
	}
}

// Start memulai server P2P.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}
	s.listener = ln

	fmt.Printf("Server P2P berjalan di %s\n", s.listenAddr)

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			fmt.Println("Error menerima koneksi:", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// blacklistPeer menambahkan IP peer ke daftar hitam.
func (s *Server) blacklistPeer(peer *Peer) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Dapatkan hanya IP, tanpa port
	ip := peer.conn.RemoteAddr().(*net.TCPAddr).IP.String()
	if _, exists := s.blacklist[ip]; !exists {
		s.blacklist[ip] = time.Now()
		fmt.Printf("Peer %s has been blacklisted.\n", ip)
	}
}

// handleConnection menangani koneksi masuk dari peer.
func (s *Server) handleConnection(conn net.Conn) {
	ip := conn.RemoteAddr().(*net.TCPAddr).IP.String()
	s.lock.RLock()
	if blacklistedAt, exists := s.blacklist[ip]; exists {
		if time.Since(blacklistedAt) > BlacklistDuration {
			s.lock.RUnlock() // Release RLock
			s.lock.Lock()    // Acquire Write Lock
			delete(s.blacklist, ip)
			s.lock.Unlock()  // Release Write Lock
			s.lock.RLock()   // Re-acquire RLock for subsequent checks
			fmt.Printf("Peer %s was blacklisted at %v, but duration expired. Allowing connection.\n", ip, blacklistedAt)
			// Continue with connection handling
		} else {
			s.lock.RUnlock()
			fmt.Printf("Refusing connection from blacklisted peer %s (blacklisted at %v, expires in %v)\n", ip, blacklistedAt, blacklistedAt.Add(BlacklistDuration).Sub(time.Now()))
			conn.Close()
			return
		}
	}
	s.lock.RUnlock()

	peer := NewPeer(conn)

	s.lock.Lock()
	s.peers[conn.RemoteAddr()] = peer
	s.lock.Unlock()

	// Untuk koneksi masuk, kita bertindak sebagai responder handshake
	if err := s.respondHandshake(peer); err != nil {
		fmt.Printf("Handshake gagal dengan %s: %v\n", conn.RemoteAddr(), err)
		s.blacklistPeer(peer)
		conn.Close()
		s.lock.Lock()
		delete(s.peers, conn.RemoteAddr())
		s.lock.Unlock()
		return
	}

	fmt.Printf("Peer baru terhubung dan handshake berhasil: %s\n", conn.RemoteAddr())
	s.readLoop(peer)
}


// respondHandshake menangani handshake dari peer yang masuk (sebagai responder).
func (s *Server) respondHandshake(peer *Peer) error {
	// Terima handshake dari peer
	handshakeMsg := &Message{}
	if err := peer.decoder.Decode(handshakeMsg); err != nil {
		return err
	}
	if handshakeMsg.Type != MessageTypeHandshake {
		return errors.New("expected handshake message on connect")
	}
	var peerHandshake HandshakePayload
	if err := gob.NewDecoder(bytes.NewReader(handshakeMsg.Payload)).Decode(&peerHandshake); err != nil {
		return err
	}
	log.Printf("Menerima handshake dari %s (version: %s, height: %d)", peer.conn.RemoteAddr(), peerHandshake.Version, peerHandshake.Height)

	// Kirim handshake kita sebagai balasan
	myHandshake := HandshakePayload{
		Version:    "swatantra-0.1",
		Height:     s.blockchain.Head().Height,
		HeadHash:   s.blockchain.Head().Hash(),
		ListenAddr: s.listenAddr,
	}
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(myHandshake); err != nil {
		return err
	}
	msg := &Message{
		Type:    MessageTypeHandshake,
		Payload: buf.Bytes(),
	}
	if err := peer.Send(msg); err != nil {
		return err
	}

	// Setelah bertukar handshake, tangani perbandingan chain
	return s.handleHandshake(peer, &peerHandshake)
}


// readLoop secara terus-menerus membaca pesan dari peer.
func (s *Server) readLoop(peer *Peer) {
	conn := peer.conn
	defer func() {
		conn.Close()
		s.lock.Lock()
		delete(s.peers, conn.RemoteAddr())
		s.lock.Unlock()
		fmt.Printf("Peer disconnected: %s\n", conn.RemoteAddr())
	}()

	for {
		if !peer.limiter.Allow() {
			fmt.Printf("Peer %s is sending messages too fast. Blacklisting and disconnecting.\n", conn.RemoteAddr())
			s.blacklistPeer(peer)
			return // Defer will handle closing and cleanup
		}

		msg := &Message{}
		if err := peer.decoder.Decode(msg); err != nil {
			// fmt.Printf("Error decoding message from %s: %v. Blacklisting and disconnecting.\n", conn.RemoteAddr(), err)
			// s.blacklistPeer(peer)
			return
		}

		s.msgCh <- &RPC{
			From:    conn.RemoteAddr(),
			Payload: msg.Payload,
			Type:    msg.Type,
		}
	}
}

// ProcessMessages Loop utama untuk memproses pesan yang masuk.
func (s *Server) ProcessMessages() {
	for rpc := range s.msgCh {
		switch rpc.Type {
		case MessageTypeTx:
			var payload TxPayload
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&payload); err != nil {
				log.Println("Error decoding TxPayload:", err)
				continue
			}
			if err := s.mempool.Add(payload.Tx); err != nil {
				// log.Println("Error adding transaction to mempool:", err)
				continue
			}
			txHash, err := payload.Tx.Hash()
			if err != nil {
				log.Println("Error getting transaction hash:", err)
				continue
			}
			log.Printf("Received new transaction: %s\n", txHash.ToHex())
			// Broadcast ke peer lain (kecuali pengirim)
			s.broadcast(rpc.Payload, rpc.Type, rpc.From)

		case MessageTypeBlock:
			var payload BlockPayload
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&payload); err != nil {
				log.Printf("P2P: Error decoding BlockPayload from %s: %v", rpc.From, err)
				continue
			}
			blockHash, _ := payload.Block.Hash()
			log.Printf("P2P: Received Block %s (height %d) from %s", blockHash.ToHex(), payload.Block.Header.Height, rpc.From)

			if err := s.blockchain.AddBlock(payload.Block); err != nil {
				// This error is now critical for debugging sync issues.
				log.Printf("P2P: Failed to add block %s from %s: %v", blockHash.ToHex(), rpc.From, err)
				continue
			}
			// Hapus transaksi dari mempool yang sudah masuk block
			for _, tx := range payload.Block.Transactions {
				txHash, _ := tx.Hash()
				s.mempool.Remove(txHash)
			}
			// Broadcast ke peer lain (kecuali pengirim)
			s.broadcast(rpc.Payload, rpc.Type, rpc.From)
		case MessageTypeGetBlocks:
			var payload GetBlocksPayload
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&payload); err != nil {
				log.Printf("P2P: Error decoding GetBlocksPayload from %s: %v", rpc.From, err)
				continue
			}
			log.Printf("P2P: Received GetBlocks request from %s (from_hash: %s)", rpc.From, payload.From.ToHex())

			// Temukan block yang diminta
			blocks, err := s.blockchain.GetBlocksFrom(payload.From)
			if err != nil {
				log.Println("Error getting blocks from blockchain:", err)
				continue
			}

			log.Printf("P2P: Found %d blocks to send to %s", len(blocks), rpc.From)

			// Kirim block kembali ke pengirim
			for _, block := range blocks {
				blockPayload := BlockPayload{Block: block}
				buf := new(bytes.Buffer)
				if err := gob.NewEncoder(buf).Encode(blockPayload); err != nil {
					log.Println("Error encoding block payload:", err)
					continue
				}
				msg := &Message{
					Type:    MessageTypeBlock,
					Payload: buf.Bytes(),
				}
				peer, ok := s.peers[rpc.From]
				if !ok {
					log.Println("Sender peer not found:", rpc.From)
					continue
				}
				if err := peer.Send(msg); err != nil {
					log.Println("Error sending block to peer:", err)
				}
			}
		// NOTE: Handshake logic is now handled directly in initiate/respond handshake funcs
		// and is no longer processed via the message channel.
		}
	}
}

// BroadcastBlock mengirimkan block ke semua peer.
func (s *Server) BroadcastBlock(b *core.Block) error {
	payload := BlockPayload{Block: b}
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(payload); err != nil {
		return err
	}

	msg := &Message{
		Type:    MessageTypeBlock,
		Payload: buf.Bytes(),
	}

	return s.broadcast(msg.Payload, msg.Type, nil)
}

// broadcast mengirim pesan ke semua peer kecuali excludeAddr.
func (s *Server) broadcast(payload []byte, msgType MessageType, excludeAddr net.Addr) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	for addr, peer := range s.peers {
		if addr == excludeAddr {
			continue
		}
		msg := &Message{
			Type:    msgType,
			Payload: payload,
		}
		if err := peer.Send(msg); err != nil {
			// Mungkin peer sudah disconnect, bisa diabaikan atau di-log
		}
	}
	return nil
}

// Connect mencoba terhubung ke peer lain.
func (s *Server) Connect(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	peer := NewPeer(conn)

	s.lock.Lock()
	s.peers[conn.RemoteAddr()] = peer
	s.lock.Unlock()

	log.Printf("Terhubung ke peer: %s", conn.RemoteAddr())

	// Lakukan handshake sebagai inisiator
	if err := s.initiateHandshake(peer); err != nil {
		log.Printf("Handshake gagal dengan %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		s.lock.Lock()
		delete(s.peers, conn.RemoteAddr())
		s.lock.Unlock()
		return err
	}

	go s.readLoop(peer)

	return nil
}

// initiateHandshake memulai proses handshake dengan peer (sebagai inisiator).
func (s *Server) initiateHandshake(peer *Peer) error {
	// Kirim handshake kita
	myHandshake := HandshakePayload{
		Version:    "swatantra-0.1",
		Height:     s.blockchain.Head().Height,
		HeadHash:   s.blockchain.Head().Hash(),
		ListenAddr: s.listenAddr,
	}
	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(myHandshake); err != nil {
		return err
	}
	msg := &Message{
		Type:    MessageTypeHandshake,
		Payload: buf.Bytes(),
	}
	if err := peer.Send(msg); err != nil {
		return err
	}

	// Terima handshake dari peer
	responseMsg := &Message{}
	if err := peer.decoder.Decode(responseMsg); err != nil {
		return err
	}
	if responseMsg.Type != MessageTypeHandshake {
		return errors.New("expected handshake message")
	}
	var peerHandshake HandshakePayload
	if err := gob.NewDecoder(bytes.NewReader(responseMsg.Payload)).Decode(&peerHandshake); err != nil {
		return err
	}

	log.Printf("Handshake berhasil dengan %s (version: %s, height: %d)", peer.conn.RemoteAddr(), peerHandshake.Version, peerHandshake.Height)

	// Setelah bertukar handshake, tangani perbandingan chain
	return s.handleHandshake(peer, &peerHandshake)
}

// handleHandshake contains the logic for comparing chain heights and syncing.
func (s *Server) handleHandshake(peer *Peer, payload *HandshakePayload) error {
	// Bandingkan tinggi chain
	if payload.Height > s.blockchain.Head().Height {
		// Peer memiliki chain yang lebih panjang, minta block dari mereka
		log.Printf("P2P: Peer %s has a longer chain (height %d > our %d). Requesting blocks.", peer.conn.RemoteAddr(), payload.Height, s.blockchain.Head().Height)

		getBlocksPayload := GetBlocksPayload{
			// Minta block mulai dari block teratas yang kita punya
			From: s.blockchain.Head().Hash(),
		}
		buf := new(bytes.Buffer)
		if err := gob.NewEncoder(buf).Encode(getBlocksPayload); err != nil {
			return err
		}
		msg := &Message{
			Type:    MessageTypeGetBlocks,
			Payload: buf.Bytes(),
		}
		return peer.Send(msg)

	} else if payload.Height < s.blockchain.Head().Height {
		// Kita memiliki chain yang lebih panjang, kirim block kita ke peer
		log.Printf("P2P: Our chain is longer (height %d > peer %d). Sending blocks to %s.", s.blockchain.Head().Height, payload.Height, peer.conn.RemoteAddr())

		blocksToSend, err := s.blockchain.GetBlocksFrom(payload.HeadHash)
		if err != nil {
			return err
		}

		log.Printf("P2P: Found %d blocks to send to %s", len(blocksToSend), peer.conn.RemoteAddr())

		for _, block := range blocksToSend {
			blockPayload := BlockPayload{Block: block}
			buf := new(bytes.Buffer)
			if err := gob.NewEncoder(buf).Encode(blockPayload); err != nil {
				return err
			}
			msg := &Message{
					Type:    MessageTypeBlock,
					Payload: buf.Bytes(),
				}
			if err := peer.Send(msg); err != nil {
				blockHash, _ := block.Hash() // Safely get hash for logging
				return fmt.Errorf("error sending block %s to peer %s: %v", blockHash.ToHex(), peer.conn.RemoteAddr(), err)
			}
		}
	}
	// If heights are equal, do nothing.
	return nil
}