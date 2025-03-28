#!/bin/bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Error: This installer must be run as root." >&2
  exit 1
fi

echo "Stopping Tor service..."
systemctl stop tor

echo "Removing existing hidden service data..."
rm -rf /var/lib/tor/hidden_service_onionscan/

echo "Ensuring /etc/tor exists..."
mkdir -p /etc/tor

echo "Writing new Tor configuration to /etc/tor/torrc..."
cat > /etc/tor/torrc << 'EOF'
SocksPort 9050

HiddenServiceDir /var/lib/tor/hidden_service_onionscan/
HiddenServicePort 1234 127.0.0.1:1234
EOF

echo "Ensuring hidden service directory exists and setting secure permissions..."
mkdir -p /var/lib/tor/hidden_service_onionscan/
chown -R debian-tor:debian-tor /var/lib/tor/hidden_service_onionscan/
chmod -R 700 /var/lib/tor/hidden_service_onionscan/

echo "Verifying Tor configuration as debian-tor..."
if ! sudo -u debian-tor tor --verify-config; then
  echo "Tor configuration verification failed." >&2
  exit 1
fi

echo "Restarting Tor service..."
systemctl restart tor

echo "Waiting 60 seconds for Tor to initialize the hidden service..."
sleep 60

if [ -f /var/lib/tor/hidden_service_onionscan/hostname ]; then
    echo "Hidden service hostname:"
    cat /var/lib/tor/hidden_service_onionscan/hostname
else
    echo "Error: Hidden service hostname file was not created."
    echo "Please check Tor logs with: journalctl -u tor -e | tail -n 50"
    exit 1
fi

echo "Tor installation and hidden service configuration completed successfully."

PROJECT_DIR="/app/CloudStorm"
NODE_PORT="3000"
GO_PORT="5115"
GO_BINARY_NAME="cloudstorm"
GO_SOURCE_NAME="/app/CloudStorm/src/main.go"

cd "$PROJECT_DIR"

if [ ! -f go.mod ]; then
  echo "[+] Initializing Go modules..."
  go mod init github.com/sudocoinxrpl/CloudStorm
fi

echo "[+] Retrieving required Go modules..."
go get github.com/fsnotify/fsnotify
go get github.com/gorilla/websocket
go get github.com/ipfs/go-ipfs-api
go get github.com/skip2/go-qrcode
go get golang.org/x/net/proxy

go mod tidy

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
