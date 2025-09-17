package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"swatantra/crypto"
)

// NodeConfig holds configuration for a test node
type NodeConfig struct {
	ListenPort int
	APIPort    int
	IsMiner    bool
	CoinbaseAddr string
	Peers      []string
	DataDir    string
	Cmd        *exec.Cmd
	ExecutablePath string // New field
}

// startNode starts a swatantra-node process
func startNode(t *testing.T, cfg NodeConfig) {
	// Create a temporary data directory for the node
	dataDir, err := ioutil.TempDir("", fmt.Sprintf("swatantra-node-data-%d-", cfg.ListenPort))
	if err != nil {
		t.Fatalf("Failed to create data directory for node %d: %v", cfg.ListenPort, err)
	}
	cfg.DataDir = dataDir

	// Create a unique config.json for this node that matches the main app's config structure
	type TestAPIConfig struct {
		ListenAddress string `json:"listenAddress"`
	}
	type TestP2PConfig struct {
		ListenAddress string `json:"listenAddress"`
	}
	type TestConfig struct {
		API TestAPIConfig `json:"api"`
		P2P TestP2PConfig `json:"p2p"`
	}

	nodeConfigFile := TestConfig{
		API: TestAPIConfig{
			ListenAddress: fmt.Sprintf(":%d", cfg.APIPort),
		},
		P2P: TestP2PConfig{
			ListenAddress: fmt.Sprintf(":%d", cfg.ListenPort),
		},
	}

	configData, err := json.Marshal(nodeConfigFile)
	if err != nil {
		t.Fatalf("Failed to marshal config for node %d: %v", cfg.ListenPort, err)
	}
	configPath := filepath.Join(dataDir, "config.json")
	if err := ioutil.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatalf("Failed to write config for node %d: %v", cfg.ListenPort, err)
	}

	args := []string{
		"start-node",
		"--config", configPath, // Use the unique config file
		"--datadir", dataDir,
	}

	if cfg.IsMiner {
		args = append(args, "--mine")
		if cfg.CoinbaseAddr != "" {
			args = append(args, "--coinbase", cfg.CoinbaseAddr)
		}
	}

	if len(cfg.Peers) > 0 {
		args = append(args, "--peers", strings.Join(cfg.Peers, ","))
	}

	cmd := exec.Command(cfg.ExecutablePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start node %d: %v", cfg.ListenPort, err)
	}
	cfg.Cmd = cmd

	t.Logf("Node %d started with PID %d in data dir %s", cfg.ListenPort, cmd.Process.Pid, cfg.DataDir)

	// Give the node some time to start up
	time.Sleep(2 * time.Second)
}

// stopNode stops a swatantra-node process and cleans up its data directory
func stopNode(t *testing.T, cfg NodeConfig) {
	if cfg.Cmd != nil && cfg.Cmd.Process != nil {
		t.Logf("Stopping node %d (PID %d)", cfg.ListenPort, cfg.Cmd.Process.Pid)
		if err := cfg.Cmd.Process.Kill(); err != nil {
			t.Errorf("Failed to kill node %d process: %v", cfg.ListenPort, err)
		}
		cfg.Cmd.Wait() // Wait for the process to exit
	}
	if cfg.DataDir != "" {
		t.Logf("Cleaning up data directory %s", cfg.DataDir)
		if err := os.RemoveAll(cfg.DataDir); err != nil {
			t.Errorf("Failed to remove data directory %s: %v", cfg.DataDir, err)
		}
	}
}

// queryAPI makes an HTTP GET request to a node's API and decodes the JSON response
func queryAPI(t *testing.T, apiPort int, endpoint string, response interface{}) error {
	url := fmt.Sprintf("http://localhost:%d%s", apiPort, endpoint)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to query API %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("API returned non-OK status %d: %s", resp.StatusCode, string(body))
	}

	if response != nil {
		return json.NewDecoder(resp.Body).Decode(response)
	}
	return nil
}

// sendTransaction sends a transaction via a node's API
func sendTransaction(t *testing.T, apiPort int, toAddress string, amount uint64, executablePath string) error { // Added executablePath parameter
	// This requires the `send-tx` CLI command to be available and working
	// For a true integration test, we might want to simulate the CLI call
	// or directly use the API if it exposes a /sendtx endpoint.

	// For simplicity, let's assume the `send-tx` CLI command is used.
	// This will require `wallet.key` to be present in the current directory where the test is run.
	// This is a limitation for multi-node tests if each node needs its own wallet.

	// A better approach would be to expose a /sendtx API endpoint that takes raw transaction data.

	// For now, we'll simulate the CLI call, assuming `wallet.key` is in the test execution directory.
	cmd := exec.Command(executablePath, "send-tx", // Use executablePath
		"--to", toAddress,
		"--amount", strconv.FormatUint(amount, 10),
		"--apiport", fmt.Sprintf(":%d", apiPort),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// cmd.Dir is not set, so it runs in the current test directory

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send transaction via CLI: %v", err)
	}
	return nil
}

// TestIntegration_TransactionFlow tests the end-to-end transaction flow across multiple nodes
func TestIntegration_TransactionFlow(t *testing.T) {
	// Build the node executable first in the current test directory
	executableName := "./swatantra-node"
	buildCmd := exec.Command("go", "build", "-o", executableName, "../../cmd/node") // Output to current directory
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build swatantra-node executable: %v", err)
	}
	
	executablePath, err := filepath.Abs(executableName)
	if err != nil {
		t.Fatalf("Failed to get absolute path of executable: %v", err)
	}

	// Generate a wallet for the miner
	minerPrivKey, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate miner private key: %v", err)
	}
	minerAddr := minerPrivKey.Public().Address().ToHex()

	// Write miner's wallet to a temporary file for the test
	// This is a workaround as `send-tx` expects `wallet.key` in the current directory.
	// For a robust test, `send-tx` should take a wallet path.
	minerWalletPath := filepath.Join(os.TempDir(), "test_miner_wallet.key")
	if err := ioutil.WriteFile(minerWalletPath, minerPrivKey, 0600); err != nil {
		t.Fatalf("Failed to write miner wallet file: %v", err)
	}
	defer os.Remove(minerWalletPath)

	// Temporarily change current directory to where the wallet is for `send-tx`
	originalDir, _ := os.Getwd()
	if err := os.Chdir(filepath.Dir(minerWalletPath)); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(originalDir) // Restore original directory


	// Node configurations
	nodeAConfig := NodeConfig{
		ListenPort: 3000,
		APIPort:    4000,
		IsMiner:    true,
		CoinbaseAddr: minerAddr,
		ExecutablePath: executablePath, // Pass executable path
	}
	nodeBConfig := NodeConfig{
		ListenPort: 3001,
		APIPort:    4001,
		Peers:      []string{fmt.Sprintf("127.0.0.1:%d", nodeAConfig.ListenPort)},
		ExecutablePath: executablePath, // Pass executable path
	}

	// Start Node A (Miner)
	startNode(t, nodeAConfig)
	defer stopNode(t, nodeAConfig)

	// Start Node B
	startNode(t, nodeBConfig)
	defer stopNode(t, nodeBConfig)

	// Give nodes time to connect and for miner to produce some blocks
	t.Log("Waiting for nodes to connect and miner to produce blocks...")
	time.Sleep(10 * time.Second)

	// Additional sleep to ensure API servers are fully ready
	time.Sleep(1 * time.Second)

	// Verify initial state: Node A should have mined some blocks
	var statusA struct { Height int `json:"height"` }
	if err := queryAPI(t, nodeAConfig.APIPort, "/status", &statusA); err != nil {
		t.Fatalf("Failed to get status from Node A: %v", err)
	}
	t.Logf("Node A initial height: %d", statusA.Height)
	if statusA.Height < 1 {
		t.Fatal("Node A did not mine any blocks")
	}

	// Verify Node B has synced
	var statusB struct { Height int `json:"height"` }
	if err := queryAPI(t, nodeBConfig.APIPort, "/status", &statusB); err != nil {
		t.Fatalf("Failed to get status from Node B: %v", err)
	}
	t.Logf("Node B initial height: %d", statusB.Height)
	if statusB.Height != statusA.Height {
		t.Fatalf("Node B did not sync with Node A. Expected height %d, got %d", statusA.Height, statusB.Height)
	}

	// Generate a wallet for the receiver
	receiverPrivKey, err := crypto.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Failed to generate receiver private key: %v", err)
	}
	receiverAddr := receiverPrivKey.Public().Address().ToHex()

	// Send a transaction from miner to receiver
	// This assumes the miner has enough funds from coinbase transactions
	t.Logf("Sending transaction from %s to %s", minerAddr, receiverAddr)
	if err := sendTransaction(t, nodeAConfig.APIPort, receiverAddr, 10, executablePath); err != nil { // Pass executablePath
		t.Fatalf("Failed to send transaction: %v", err)
	}

	// Give time for transaction to be included in a block
	t.Log("Waiting for transaction to be included in a block...")
	time.Sleep(5 * time.Second)

	// Verify transaction is in a new block and propagated
	var newStatusA struct { Height int `json:"height"` }
	if err := queryAPI(t, nodeAConfig.APIPort, "/status", &newStatusA); err != nil {
		t.Fatalf("Failed to get status from Node A after tx: %v", err)
	}
	var newStatusB struct { Height int `json:"height"` }
	if err := queryAPI(t, nodeBConfig.APIPort, "/status", &newStatusB); err != nil {
		t.Fatalf("Failed to get status from Node B after tx: %v", err)
	}

	t.Logf("Node A height after tx: %d", newStatusA.Height)
	t.Logf("Node B height after tx: %d", newStatusB.Height)

	if newStatusA.Height <= statusA.Height {
		t.Fatal("Node A did not mine a new block after transaction")
	}
	if newStatusB.Height != newStatusA.Height {
		t.Fatalf("Node B did not sync new block. Expected height %d, got %d", newStatusA.Height, newStatusB.Height)
	}

	// Further verification: check UTXO balances (requires API endpoints for UTXOs/balances)
	// This part is commented out as the API for UTXOs/balances is in Phase 7.
	// For now, we rely on block height increase and sync.

	t.Log("Transaction flow integration test completed successfully.")
}

// TestIntegration_ForkAndReorg simulates a blockchain fork and verifies that a reorg occurs.
func TestIntegration_ForkAndReorg(t *testing.T) {
	// Use a unique executable name to avoid race conditions if tests run in parallel
	executableName := "./swatantra-node-fork-test"
	buildCmd := exec.Command("go", "build", "-o", executableName, "../../cmd/node")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build swatantra-node executable: %v\n%s", err, string(output))
	}
	executablePath, err := filepath.Abs(executableName)
	if err != nil {
		t.Fatalf("Failed to get absolute path of executable: %v", err)
	}
	defer os.Remove(executablePath)

	// --- Phase 1: Initial Setup & Sync ---
	minerAWallet, _ := crypto.GeneratePrivateKey()
	minerAAddr := minerAWallet.Public().Address().ToHex()

	// Node configurations
	nodeA := NodeConfig{ListenPort: 5000, APIPort: 6000, IsMiner: true, CoinbaseAddr: minerAAddr, ExecutablePath: executablePath}
	nodeB := NodeConfig{ListenPort: 5001, APIPort: 6001, Peers: []string{"127.0.0.1:5000"}, ExecutablePath: executablePath}
	nodeC := NodeConfig{ListenPort: 5002, APIPort: 6002, Peers: []string{"127.0.0.1:5000"}, ExecutablePath: executablePath}

	// Start all nodes, with A as the only miner initially
	startNode(t, nodeA)
	defer stopNode(t, nodeA)
	startNode(t, nodeB)
	defer stopNode(t, nodeB)
	startNode(t, nodeC)
	defer stopNode(t, nodeC)

	t.Log("Phase 1: Waiting for all nodes to sync to a common height...")
	time.Sleep(10 * time.Second) // Wait for A to mine a few blocks

	// Additional sleep to ensure API servers are fully ready
	time.Sleep(1 * time.Second)

	// Verify initial sync
	var statusA, statusB, statusC struct {
		Height int
		Head   string
	}
	if err := queryAPI(t, nodeA.APIPort, "/status", &statusA); err != nil {
		t.Fatalf("Failed to query Node A: %v", err)
	}
	if err := queryAPI(t, nodeB.APIPort, "/status", &statusB); err != nil {
		t.Fatalf("Failed to query Node B: %v", err)
	}
	if err := queryAPI(t, nodeC.APIPort, "/status", &statusC); err != nil {
		t.Fatalf("Failed to query Node C: %v", err)
	}

	if statusA.Height < 2 {
		t.Fatal("Node A failed to mine sufficient initial blocks")
	}
	if !(statusA.Height == statusB.Height && statusB.Height == statusC.Height) {
		t.Fatalf("Initial sync failed. Heights: A=%d, B=%d, C=%d", statusA.Height, statusB.Height, statusC.Height)
	}
	if !(statusA.Head == statusB.Head && statusB.Head == statusC.Head) {
		t.Fatalf("Initial sync failed. Head hashes do not match.")
	}
	commonHeight := statusA.Height
	t.Logf("Phase 1: Sync successful at height %d", commonHeight)

	// --- Phase 2: Create Network Partition ---
	t.Log("Phase 2: Creating network partition. (A) and (C) will be isolated miners.")
	stopNode(t, nodeA)
	stopNode(t, nodeB) // B is just a follower, stop it for now.
	stopNode(t, nodeC)

	// Restart A as an isolated miner
	nodeA.Peers = []string{}
	startNode(t, nodeA)

	// Restart C as an isolated miner
	nodeC.Peers = []string{}
	nodeC.IsMiner = true
	nodeC.CoinbaseAddr = minerAAddr // Can be same or different
	startNode(t, nodeC)

	// --- Phase 3: Create Diverging Chains (Fork) ---
	t.Log("Phase 3: Mining on separate forks...")
	// Let Node A mine 1 block
	t.Log("Fork A mining 1 block...")
	time.Sleep(5 * time.Second)

	// Let Node C mine 2 blocks to create a longer chain
	t.Log("Fork C mining 2 blocks...")
	time.Sleep(10 * time.Second)

	// Additional sleep to ensure API servers are fully ready
	time.Sleep(1 * time.Second)

	// Verify fork state
	if err := queryAPI(t, nodeA.APIPort, "/status", &statusA); err != nil {
		t.Fatalf("Failed to query Node A during fork: %v", err)
	}
	if err := queryAPI(t, nodeC.APIPort, "/status", &statusC); err != nil {
		t.Fatalf("Failed to query Node C during fork: %v", err)
	}

	if statusA.Height <= commonHeight {
		t.Errorf("Fork A did not grow. Expected height > %d, got %d", commonHeight, statusA.Height)
	}
	if statusC.Height <= statusA.Height {
		t.Errorf("Fork C is not longer than Fork A. Expected C height > %d, got %d", statusA.Height, statusC.Height)
	}
	if statusA.Head == statusC.Head {
		t.Fatal("Forks have identical head hashes, fork failed.")
	}
	t.Logf("Phase 3: Forks created. Fork A height: %d, Fork C height: %d", statusA.Height, statusC.Height)
	forkCHead := statusC.Head

	// --- Phase 4: Heal Partition & Trigger Reorg ---
	t.Log("Phase 4: Healing partition between A and C to trigger reorg on A.")
	stopNode(t, nodeA)
	// Restart A, now peered with C
	nodeA.Peers = []string{"127.0.0.1:5002"}
	nodeA.IsMiner = false // Stop A from mining to clearly see the reorg
	startNode(t, nodeA)

	t.Log("Waiting for Node A to sync with Node C and reorg...")
	time.Sleep(8 * time.Second)

	// Additional sleep to ensure API servers are fully ready
	time.Sleep(1 * time.Second)

	// --- Phase 5: Verification ---
	t.Log("Phase 5: Verifying reorg...")
	if err := queryAPI(t, nodeA.APIPort, "/status", &statusA); err != nil {
		t.Fatalf("Failed to query Node A post-reorg: %v", err)
	}
	if err := queryAPI(t, nodeC.APIPort, "/status", &statusC); err != nil {
		t.Fatalf("Failed to query Node C post-reorg: %v", err)
	}

	if statusA.Height != statusC.Height {
		t.Fatalf("Reorg failed for Node A. Expected height %d, got %d", statusC.Height, statusA.Height)
	}
	if statusA.Head != forkCHead {
		t.Fatalf("Reorg failed for Node A. Head hash does not match Fork C's head. Got %s, expected %s", statusA.Head, forkCHead)
	}

	t.Logf("Phase 5: Node A successfully reorged to height %d with head %s", statusA.Height, statusA.Head)
	t.Log("Fork and Reorg integration test completed successfully.")
}