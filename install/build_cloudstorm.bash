#!/bin/bash
set -e
PROJECT_DIR="/app/CloudStorm"
NODE_PORT="3000"
GO_PORT="5115"
GO_BINARY_NAME="cloudstorm"
GO_SOURCE_NAME="/app/CloudStorm/src/main.go"
cd "$PROJECT_DIR"

echo "[+] Installing Node.js dependencies..."
npm install

echo "[+] Building '$GO_BINARY_NAME' (Go) in $PROJECT_DIR..."
if ! go build -o "$GO_BINARY_NAME" "$GO_SOURCE_NAME"; then
  echo "[!] ERROR: Go build failed."
  exit 1
fi
echo "[+] Build succeeded. Binary => $PROJECT_DIR/$GO_BINARY_NAME"

echo "[+] Starting Cloudstorm (Go server) on port $GO_PORT with logging to go_server.log..."
nohup "$PROJECT_DIR/$GO_BINARY_NAME" --port="$GO_PORT" --ipfs="ipfs_container:5001" --basedir="." > go_server.log 2>&1 &
sleep 4
echo "----- Last 50 lines of go_server.log -----"
tail -n 50 go_server.log || true
echo "------------------------------------------"

if ! pgrep -f "$PROJECT_DIR/$GO_BINARY_NAME --port=$GO_PORT" >/dev/null; then
  echo "[!] '$GO_BINARY_NAME' is not running. Check go_server.log."
  exit 1
fi

echo "[+] Starting Node server on port $NODE_PORT..."
API_PORT=$NODE_PORT npm start
