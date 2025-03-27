package main

import (
	"CloudStorm/fswatch"
	"CloudStorm/ipfs"
	jwtutil "CloudStorm/jwt"
	"CloudStorm/raft"
	trinity "CloudStorm/trinitygo"
	"CloudStorm/wallet"
	"CloudStorm/ws"

	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func startNodeServer() {
	// Launch the Node.js server (server.js) as a separate process.
	cmd := exec.Command("node", "webapp/server.js")
	// Pipe stdout and stderr for logging.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start node server: %v", err)
	}
	log.Printf("Node server started with PID %d", cmd.Process.Pid)

	// Optionally, wait for the process to exit in a goroutine.
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Node server exited with error: %v", err)
		} else {
			log.Printf("Node server exited normally")
		}
	}()
}

func main() {
	// Start the Node.js server so that it runs concurrently with our Go node.
	startNodeServer()

	// --- Flags ---
	ipfsAddr := flag.String("ipfs", "ipfs_container:5001", "IPFS API endpoint")
	baseDir := flag.String("basedir", ".", "Directory for computing serviceID")
	peersArg := flag.String("peers", "", "Comma-separated list of peer addresses")
	dbPath := flag.String("db", "cloudstorm.db", "Local BoltDB path for Raft node data")
	nodeID := flag.String("nodeid", "NodeA", "Unique node ID for this Raft node")
	useIBTAllPorts := flag.Bool("allports", false, "Use iBT all-port distance if true")
	// Example dimension config for iBT: 2D iBT with 32 nodes in first dimension, 16 in second
	dims := []raft.IBTDimension{
		{Size: 32, BypassSchemes: []int{8, 12}},
		{Size: 16, BypassSchemes: []int{4}},
	}
	flag.Parse()

	// Start the Trinity server (from the trinitygo module) in a separate goroutine.
	// This server will dynamically discover active serviceIDs and expose the /consensus endpoint.
	go trinity.RunTrinityServerBlocking(*baseDir)

	// --------------------------------------------------------
	// IPFS client initialization
	// --------------------------------------------------------
	ipfsClient := ipfs.NewClient(*ipfsAddr)
	_ = ipfsClient

	// --------------------------------------------------------
	// Generate a local Ripple wallet
	// --------------------------------------------------------
	address, recovery, err := wallet.GenerateRippleWallet()
	if err != nil {
		log.Fatalf("Failed to generate wallet: %v", err)
	}
	fmt.Println("Generated wallet address:", address)
	fmt.Println("Recovery key:", recovery)

	// --------------------------------------------------------
	// Compute and display the initial ServiceID using the trinitygo module.
	// --------------------------------------------------------
	serviceID, err := trinity.ComputeServiceID(*baseDir)
	if err != nil {
		log.Fatalf("Failed to compute ServiceID: %v", err)
	}
	fmt.Println("Initial ServiceID:", serviceID)

	// --------------------------------------------------------
	// Set up the Raft node
	// --------------------------------------------------------
	var peerList []string
	if *peersArg != "" {
		peerList = strings.Split(*peersArg, ",")
	}
	// For demonstration, we pass nil for TLS config.
	tlsCfg := (*tls.Config)(nil)
	node, err := raft.NewRaftNode(
		*nodeID,
		peerList,
		*dbPath,
		tlsCfg,
		dims,
		*useIBTAllPorts,
	)
	if err != nil {
		log.Fatalf("Error creating RaftNode: %v", err)
	}
	// Set the global Raft node.
	raft.SetGlobalNode(node)
	node.Start() // start the node's internal goroutine

	// --------------------------------------------------------
	// Set up JWT endpoint and WebSocket handler.
	// --------------------------------------------------------
	http.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		token, err := jwtutil.GenerateToken("user")
		if err != nil {
			http.Error(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(token))
	})
	http.HandleFunc("/ws", ws.WsHandler)

	// --------------------------------------------------------
	// File-watch for Trinity updates.
	// --------------------------------------------------------
	updateChan := make(chan string)
	go fswatch.WatchForUpdates(*baseDir, updateChan)
	go func() {
		for newSID := range updateChan {
			fmt.Println("ServiceID updated:", newSID)
		}
	}()

	// --------------------------------------------------------
	// Start an HTTP server for node endpoints.
	// --------------------------------------------------------
	go func() {
		addr := "0.0.0.0:3001"
		log.Println("Node server listening on", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatal(err)
		}
	}()

	// --------------------------------------------------------
	// Periodically print Raft status.
	// --------------------------------------------------------
	go func() {
		for {
			fmt.Println("Raft status:", raft.LogStatus())
			time.Sleep(10 * time.Second)
		}
	}()

	// Keep main alive.
	select {}
}
