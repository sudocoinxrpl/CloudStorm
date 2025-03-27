#!/bin/sh
set -euo pipefail
if [ "$(id -u)" -ne 0 ]; then
  echo "Error: Must run as root." >&2
  exit 1
fi
rm -rf /var/lib/tor/hidden_service_onionscan/
mkdir -p /etc/tor
cat > /etc/tor/torrc << 'EOF'
SocksPort 9050
HiddenServiceDir /var/lib/tor/hidden_service_onionscan/
HiddenServicePort 1234 127.0.0.1:1234
EOF
mkdir -p /var/lib/tor/hidden_service_onionscan/
chown -R debian-tor:debian-tor /var/lib/tor/hidden_service_onionscan/
chmod -R 700 /var/lib/tor/hidden_service_onionscan/
su -s /bin/sh debian-tor -c 'tor --verify-config'
tor --RunAsDaemon 0 &
tor_pid=$!
sleep 60
if [ -f /var/lib/tor/hidden_service_onionscan/hostname ]; then
  cat /var/lib/tor/hidden_service_onionscan/hostname
else
  echo "Error: Hidden service hostname file was not created." >&2
  exit 1
fi
PROJECT_DIR="/CloudStorm"
cd "$PROJECT_DIR"
if [ ! -f go.mod ]; then
  go mod init github.com/sudocoinxrpl/CloudStorm
fi
go get github.com/fsnotify/fsnotify
go get github.com/gorilla/websocket
go get github.com/ipfs/go-ipfs-api
go get github.com/skip2/go-qrcode
go get golang.org/x/net/proxy
go mod tidy
npm install
NODE_PORT="3000"
GO_PORT="5115"
GO_BINARY_NAME="cloudstorm"
GO_SOURCE_NAME="/CloudStorm/src/main.go"
if ! go build -o "$GO_BINARY_NAME" "$GO_SOURCE_NAME"; then
  exit 1
fi
nohup "$PROJECT_DIR/$GO_BINARY_NAME" --port="$GO_PORT" --ipfs="ipfs_container:5001" --basedir="." > go_server.log 2>&1 &
sleep 4
tail -n 50 go_server.log || true
if ! pgrep -f "$PROJECT_DIR/$GO_BINARY_NAME --port=$GO_PORT" >/dev/null; then
  exit 1
fi
API_PORT=$NODE_PORT npm start
wait $tor_pid
