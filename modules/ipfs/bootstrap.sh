#!/bin/bash
set -euo pipefail
ulimit -c unlimited
[ -w /proc/sys/kernel/core_pattern ] && echo "/tmp/core.%e.%p.%h.%t" > /proc/sys/kernel/core_pattern
NODE_ID="${HOSTNAME:-$(hostname)}"
WORKDIR="/opt/CloudStorm"
MAX_ATTEMPTS=45
TRINITY_SOCK_PATH="${TRINITY_SOCK_PATH:-/var/run/trinity.sock}"
export IPFS_PATH=/data/ipfs

echo "[+] Waiting for Trinity consensus..."
for i in $(seq 1 $MAX_ATTEMPTS); do
    if curl -s --unix-socket "$TRINITY_SOCK_PATH" http://localhost/consensus | grep -q '"proof_key_hash"'; then
        echo "[✓] Consensus achieved."
        break
    fi
    sleep 1
done

echo "[+] Generating IPFS swarm key..."
SALT=$(head -c 32 /dev/urandom | sha256sum | awk '{print $1}')
KEY=$(echo -n "$SALT" | sha512sum | awk '{print $1}' | cut -c1-64)
mkdir -p "$IPFS_PATH"
cat > "$IPFS_PATH/swarm.key" <<EOF
/key/swarm/psk/1.0.0/
/base16/
$KEY
EOF
chmod 600 "$IPFS_PATH/swarm.key"

echo "[+] Initializing IPFS repo..."
[ ! -f "$IPFS_PATH/config" ] && ipfs init

echo "[+] Configuring Tor hidden service..."
mkdir -p /etc/tor /var/lib/tor/hidden_service_ipfs
chown -R debian-tor:debian-tor /var/lib/tor/hidden_service_ipfs
chmod 700 /var/lib/tor/hidden_service_ipfs
cat > /etc/tor/torrc <<EOF
SocksPort 127.0.0.1:9050
HiddenServiceDir /var/lib/tor/hidden_service_ipfs/
HiddenServicePort 4001 127.0.0.1:4001
HiddenServicePort 5001 127.0.0.1:5001
EOF
gosu debian-tor tor --RunAsDaemon 1
sleep 10
cat /var/lib/tor/hidden_service_ipfs/hostname || echo "[!] Onion hostname missing."

echo "[+] Applying IPFS config for Tor-only..."
ipfs config --json Bootstrap '[]'
ipfs config --json Swarm.DisableNatPortMap true
ipfs config --json Swarm.EnableAutoNATService false
ipfs config --json Swarm.Transports.Network.Relay false
ipfs config --json Swarm.AddrFilters '[]'
ipfs config --json Swarm.ConnMgr.LowWater 20
ipfs config --json Swarm.ConnMgr.HighWater 40
ipfs config --json Swarm.AddrsListen '["/ip4/127.0.0.1/tcp/4001"]'
ipfs config --json Swarm.AddrsDial '["/dns4/localhost/tcp/4001"]'
ipfs config --json Addresses.Swarm '["/dns4/localhost/tcp/4001","/dns4/localhost/tcp/4001/ws"]'
ipfs config Addresses.API /ip4/127.0.0.1/tcp/5001
ipfs config Addresses.Gateway /ip4/127.0.0.1/tcp/8080
ipfs config --json Experimental.Libp2pStreamMounting false
ipfs config --json Swarm.Network.EnableRelayHop false
ipfs config --json Swarm.Network.Tor.SOCKS "socks5://127.0.0.1:9050"

echo "[+] Launching IPFS daemon..."
exec ipfs daemon --enable-gc --migrate=true
