package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"swatantra/core"
	"swatantra/crypto"
	"swatantra/mempool"
)

type APIServer struct {
	listenAddr string
	blockchain *core.Blockchain
	mempool    *mempool.Mempool
}

func NewAPIServer(listenAddr string, bc *core.Blockchain, mp *mempool.Mempool) *APIServer {
	return &APIServer{
		listenAddr: listenAddr,
		blockchain: bc,
		mempool:    mp,
	}
}

func (s *APIServer) Start() error {
	http.HandleFunc("/utxos/", s.handleGetUTXOs)
	http.HandleFunc("/tx", s.handlePostTx)
	fmt.Printf("API server running on %s\n", s.listenAddr)
	return http.ListenAndServe(s.listenAddr, nil)
}

func (s *APIServer) handleGetUTXOs(w http.ResponseWriter, r *http.Request) {
	addressHex := r.URL.Path[len("/utxos/"):]
	addressBytes, err := hex.DecodeString(addressHex)
	if err != nil {
		http.Error(w, "Invalid address format", http.StatusBadRequest)
		return
	}
	var address crypto.Address
	copy(address[:], addressBytes)

	utxos, err := s.blockchain.FindUTXOs(address)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(utxos)
}

func (s *APIServer) handlePostTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var tx core.Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.mempool.Add(&tx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Transaction added to mempool")
}
