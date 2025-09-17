package config

import (
	"encoding/json"
	"os"
)

// P2PConfig holds configuration for P2P networking.
type P2PConfig struct {
	ListenAddress string   `json:"listenAddress"`
	InitialPeers  []string `json:"initialPeers"`
}

// APIConfig holds configuration for the HTTP API.
type APIConfig struct {
	ListenAddress string `json:"listenAddress"`
}

// ChainConfig holds configuration for the blockchain.
type ChainConfig struct {
	InitialDifficulty uint32 `json:"initialDifficulty"`
	MaxBlockSize      int    `json:"maxBlockSize"`
	MempoolSize       int    `json:"mempoolSize"`
}

// Config is the main configuration structure.
type Config struct {
	P2P   P2PConfig   `json:"p2p"`
	API   APIConfig   `json:"api"`
	Chain ChainConfig `json:"chain"`
}

// Load loads the configuration from the given file path.
func Load(filePath string) (*Config, error) {
	configFile, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	var cfg Config
	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
