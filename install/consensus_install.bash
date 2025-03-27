#!/bin/bash
set -e

# Change to your project directory
PROJECT_DIR="/app/CloudStorm/"
cd "$PROJECT_DIR"

# Initialize go.mod if it doesn't exist
if [ ! -f go.mod ]; then
  echo "[+] Initializing Go modules..."
  go mod init github.com/sudocoinxrpl/CloudStorm
fi

# Get required dependencies
echo "[+] Retrieving required Go modules..."
go get github.com/fsnotify/fsnotify
go get github.com/gorilla/websocket
go get github.com/ipfs/go-ipfs-api
go get github.com/skip2/go-qrcode
go get golang.org/x/net/proxy

# Tidy up the go.mod file
go mod tidy

# Build the binary
echo "[+] Building CloudStormGo binary..."
go build -o CloudStormGo main.go
echo "[+] Build succeeded. Binary => $PROJECT_DIR/CloudStormGo"
