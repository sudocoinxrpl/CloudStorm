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
	cmd := exec.Command("node", "webapp/server.js")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start Node.js server: %v", err)
	}
	log.Printf("Node server started with PID %d", cmd.Process.Pid)
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Node server exited with error: %v", err)
		} else {
			log.Printf("Node server exited cleanly")
		}
	}()
}

func main() {
	startNodeServer()

	ipfsAddr := flag.String("ipfs", "ipfs_container:5001", "IPFS API endpoint")
	baseDir := flag.String("basedir", ".", "Directory for computing ServiceID")
	peersArg := flag.String("peers", "", "Comma-separated list of peer addresses")
	dbPath := flag.String("db", "cloudstorm.db", "Local BoltDB path")
	nodeID := flag.String("nodeid", "NodeA", "Unique Raft node ID")
	useIBTAllPorts := flag.Bool("allports", false, "Use all-port IBT routing")

	dims := []raft.IBTDimension{
		{Size: 32, BypassSchemes: []int{8, 12}},
		{Size: 16, BypassSchemes: []int{4}},
	}
	flag.Parse()

	ipfsClient := ipfs.NewClient(*ipfsAddr)
	_ = ipfsClient

	address, recovery, err := wallet.GenerateRippleWallet()
	if err != nil {
		log.Fatalf("Wallet generation failed: %v", err)
	}
	fmt.Println("Generated wallet address:", address)
	fmt.Println("Recovery key:", recovery)

	serviceID, err := trinity.ComputeServiceID(*baseDir)
	if err != nil {
		log.Fatalf("Failed to compute ServiceID: %v", err)
	}
	fmt.Println("Initial ServiceID:", serviceID)

	var peers []string
	if *peersArg != "" {
		peers = strings.Split(*peersArg, ",")
	}
	tlsCfg := (*tls.Config)(nil)

	node, err := raft.NewRaftNode(*nodeID, peers, *dbPath, tlsCfg, dims, *useIBTAllPorts)
	if err != nil {
		log.Fatalf("Raft node init failed: %v", err)
	}
	raft.SetGlobalNode(node)
	node.Start()

	http.HandleFunc("/api/token", func(w http.ResponseWriter, r *http.Request) {
		token, err := jwtutil.GenerateToken("user")
		if err != nil {
			http.Error(w, "Token generation failed", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(token))
	})

	http.HandleFunc("/ws", ws.WsHandler)

	updateChan := make(chan string)
	go fswatch.WatchForUpdates(*baseDir, updateChan)
	go func() {
		for newSID := range updateChan {
			fmt.Println("ServiceID updated:", newSID)
		}
	}()

	go func() {
		addr := "0.0.0.0:3001"
		log.Printf("HTTP server listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		for {
			fmt.Println("Raft status:", raft.LogStatus())
			time.Sleep(10 * time.Second)
		}
	}()

	select {}
}
