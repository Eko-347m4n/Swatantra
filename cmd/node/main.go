package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"swatantra/api"
	"swatantra/config"
	"swatantra/core"
	"swatantra/crypto"
	"swatantra/mempool"
	"swatantra/miner"
	"swatantra/p2p"
	"swatantra/storage"
)

var rootCmd = &cobra.Command{
	Use:   "swatantra-node",
	Short: "Node dan CLI untuk blockchain Swatantra",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Selamat datang di Swatantra Node. Gunakan --help untuk melihat command yang tersedia.")
	},
}

var createWalletCmd = &cobra.Command{
	Use:   "create-wallet",
	Short: "Membuat wallet baru dan menyimpannya ke file",
	Run: func(cmd *cobra.Command, args []string) {
		privKey, err := crypto.GeneratePrivateKey()
		if err != nil {
			fmt.Println("Error membuat private key:", err)
			os.Exit(1)
		}

		if err := os.WriteFile("wallet.key", privKey, 0600); err != nil {
			fmt.Println("Error menyimpan wallet:", err)
			os.Exit(1)
		}

		pubKey := privKey.Public()
		address := pubKey.Address()

		fmt.Println("Wallet baru berhasil dibuat!")
		fmt.Printf("Alamat: %s\n", address.ToHex())
		fmt.Println("Private key disimpan di: wallet.key")
	},
}

var startNodeCmd = &cobra.Command{
	Use:   "start-node",
	Short: "Memulai node Swatantra",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		cfg, err := config.Load(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("Config file '%s' not found, using defaults.\n", configPath)
				cfg = &config.Config{
					P2P: config.P2PConfig{
						ListenAddress: ":3000",
					},
					API: config.APIConfig{
						ListenAddress: ":4000",
					},
					Chain: config.ChainConfig{
						InitialDifficulty: 10,
						MaxBlockSize:      1048576,
						MempoolSize:       5000,
					},
				}
			} else {
				fmt.Println("Error loading config file:", err)
				os.Exit(1)
			}
		}

		listenAddr := cfg.P2P.ListenAddress
		if cmd.Flags().Changed("listen") {
			listenAddr, _ = cmd.Flags().GetString("listen")
		}

		peers := cfg.P2P.InitialPeers
		if cmd.Flags().Changed("peers") {
			peersStr, _ := cmd.Flags().GetString("peers")
			if peersStr != "" {
				peers = strings.Split(peersStr, ",")
			} else {
				peers = []string{}
			}
		}

		dataDir, _ := cmd.Flags().GetString("datadir")
		if dataDir == "" {
			dataDir = "./blockchain_db" // Default data directory
		}
		store, err := storage.NewLevelDBStore(dataDir)
		if err != nil {
			fmt.Println("Error membuka database:", err)
			os.Exit(1)
		}

		bc, err := core.NewBlockchain(store, cfg.Chain.InitialDifficulty)
		if err != nil {
			fmt.Println("Error inisialisasi blockchain:", err)
			os.Exit(1)
		}

		mp := mempool.NewMempool(bc, cfg.Chain.MempoolSize)

		apiServer := api.NewAPIServer(cfg.API.ListenAddress, bc, mp)
		go func() {
			if err := apiServer.Start(); err != nil {
				fmt.Println("Error starting API server:", err)
			}
		}()

		server := p2p.NewServer(listenAddr, bc, mp)

		go func() {
			if err := server.Start(); err != nil {
				fmt.Println("Error memulai server P2P:", err)
				os.Exit(1)
			}
		}()

		// Connect ke peers
		for _, peerAddr := range peers {
			go func(addr string) {
				if err := server.Connect(addr); err != nil {
					fmt.Printf("Error terhubung ke peer %s: %v\n", addr, err)
				}
			}(peerAddr)
		}

		if shouldMine, _ := cmd.Flags().GetBool("mine"); shouldMine {
			coinbaseStr, _ := cmd.Flags().GetString("coinbase")
			var coinbaseAddr crypto.Address
			if coinbaseStr != "" {
				addrBytes, err := hex.DecodeString(coinbaseStr)
				if err != nil {
					fmt.Println("Invalid coinbase address:", err)
					os.Exit(1)
				}
				copy(coinbaseAddr[:], addrBytes)
			} else {
				// Use address from wallet.key
				keyData, err := os.ReadFile("wallet.key")
				if err != nil {
					fmt.Println("Error reading wallet.key for coinbase address:", err)
					os.Exit(1)
				}
				privKey := crypto.PrivateKey(keyData)
				coinbaseAddr = privKey.Public().Address()
			}
			
			fmt.Printf("Mining enabled. Coinbase address: %s\n", coinbaseAddr.ToHex())
			miner := miner.NewMiner(bc, mp, server, coinbaseAddr, cfg.Chain.MaxBlockSize)
			miner.Start()
		}

		server.ProcessMessages()
	},
}

var sendTxCmd = &cobra.Command{
	Use:   "send-tx",
	Short: "Kirim transaksi dari wallet Anda",
	Run: func(cmd *cobra.Command, args []string) {
		toStr, _ := cmd.Flags().GetString("to")
		amount, _ := cmd.Flags().GetUint64("amount")
		apiPort, _ := cmd.Flags().GetString("apiport")

		// 1. Decode recipient address
	
toAddrBytes, err := hex.DecodeString(toStr)
		if err != nil {
			fmt.Println("Error decoding recipient address:", err)
			os.Exit(1)
		}
		var toAddr crypto.Address
		copy(toAddr[:], toAddrBytes)

		// 2. Read wallet
		keyData, err := os.ReadFile("wallet.key")
		if err != nil {
			fmt.Println("Error reading wallet.key:", err)
			os.Exit(1)
		}
		privKey := crypto.PrivateKey(keyData)
		myAddress := privKey.Public().Address()
		fmt.Printf("My address: %s\n", myAddress.ToHex())

		// 3. Get UTXOs from API
		apiURL := fmt.Sprintf("http://localhost%s/utxos/%s", apiPort, myAddress.ToHex())
		resp, err := http.Get(apiURL)
		if err != nil {
			fmt.Println("Error getting UTXOs from node:", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			fmt.Printf("Error from node API: %s\n", string(body))
			os.Exit(1)
		}

		var utxos []*core.SpentUTXO
		if err := json.NewDecoder(resp.Body).Decode(&utxos); err != nil {
			fmt.Println("Error decoding UTXOs:", err)
			os.Exit(1)
		}

		// 4. Select UTXOs and create inputs
		var inputs []*core.TxInput
		var totalInputAmount uint64 = 0
		for _, utxo := range utxos {
			inputs = append(inputs, &core.TxInput{
				PrevTxHash:   utxo.TxHash,
				PrevOutIndex: utxo.Index,
			})
			totalInputAmount += utxo.Output.Value
			if totalInputAmount >= amount {
				break
			}
		}

		if totalInputAmount < amount {
			fmt.Printf("Insufficient funds. Have %d, need %d\n", totalInputAmount, amount)
			os.Exit(1)
		}

		// 5. Create outputs
		var outputs []*core.TxOutput
		outputs = append(outputs, &core.TxOutput{
			Value:   amount,
			Address: toAddr,
		})

		// Handle change
		if totalInputAmount > amount {
			outputs = append(outputs, &core.TxOutput{
				Value:   totalInputAmount - amount,
				Address: myAddress,
			})
		}

		// 6. Create and sign transaction
		tx := core.NewTransaction(inputs, outputs)
		if err := tx.Sign(privKey); err != nil {
			fmt.Println("Error signing transaction:", err)
			os.Exit(1)
		}

		// 7. Send transaction to API
		txBytes, err := json.Marshal(tx)
		if err != nil {
			fmt.Println("Error marshalling transaction:", err)
			os.Exit(1)
		}

		postURL := fmt.Sprintf("http://localhost%s/tx", apiPort)
		postResp, err := http.Post(postURL, "application/json", bytes.NewReader(txBytes))
		if err != nil {
			fmt.Println("Error sending transaction to node:", err)
			os.Exit(1)
		}
		defer postResp.Body.Close()

		body, _ := io.ReadAll(postResp.Body)
		fmt.Printf("Server response: %s\n", string(body))
	},
}


func init() {
	rootCmd.AddCommand(createWalletCmd)
	rootCmd.AddCommand(startNodeCmd)
	rootCmd.AddCommand(sendTxCmd)

	startNodeCmd.Flags().String("listen", "", "Alamat untuk mendengarkan koneksi P2P (override config)")
	startNodeCmd.Flags().String("peers", "", "Daftar alamat peer untuk dihubungi (override config, dipisahkan koma)")
	startNodeCmd.Flags().String("config", "./config/config.json", "Path ke file konfigurasi JSON")
	startNodeCmd.Flags().Bool("mine", false, "Aktifkan mode mining")
	startNodeCmd.Flags().String("coinbase", "", "Alamat untuk menerima reward mining (default: dari wallet.key)")
	startNodeCmd.Flags().String("datadir", "", "Direktori untuk menyimpan data blockchain (default: ./blockchain_db)")

	sendTxCmd.Flags().String("to", "", "Alamat penerima")
	sendTxCmd.Flags().Uint64("amount", 0, "Jumlah yang akan dikirim")
	sendTxCmd.Flags().String("apiport", ":4000", "Port API node yang sedang berjalan")
	sendTxCmd.MarkFlagRequired("to")
	sendTxCmd.MarkFlagRequired("amount")
}


func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}