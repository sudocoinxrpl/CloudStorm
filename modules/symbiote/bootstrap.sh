#!/bin/bash
set -euo pipefail

ulimit -c unlimited
if [ -w /proc/sys/kernel/core_pattern ]; then
    echo "/tmp/core.%e.%p.%h.%t" > /proc/sys/kernel/core_pattern
fi

# -------------------------------------------------------------------------------
# Symbiote Module Bootstrap
# -------------------------------------------------------------------------------
NODE_ID="${HOSTNAME:-$(hostname)}"
WORKDIR="/opt/CloudStorm"
TRINITY_SOCK_PATH="${TRINITY_SOCK_PATH:-/var/run/trinity.sock}"
MAX_ATTEMPTS=45

echo "[debug] Node ID: $NODE_ID"

# Stage 0: Ensure Trinity consensus
echo "[+] Waiting for Trinity consensus lock..."
for i in $(seq 1 $MAX_ATTEMPTS); do
    if curl --silent --unix-socket "$TRINITY_SOCK_PATH" http://localhost/consensus | grep -q '"proof_key_hash"'; then
        echo "[✓] Trinity consensus achieved."
        break
    fi
    sleep 1
    echo "  retry $i..."
done

# Unset any proxy so rippled talks directly
# security impact of this requirement needs to looked at
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY

# -------------------------------------------------------------------------------
# Stage 1: Launch Rippled
# -------------------------------------------------------------------------------
echo "[+] Launching rippled..."
rippled --conf /opt/rippled/rippled.cfg &
sleep 5
if ! pgrep -f rippled >/dev/null; then
    echo "[!] rippled did not start."
    exit 1
fi

echo "[+] Waiting for at least one XRPL peer..."
for i in $(seq 1 30); do
    peers=$(rippled --quiet json peer_info | jq '.result.peers | length')
    if [ "$peers" -ge 1 ]; then
        echo "[✓] Connected to $peers peer(s)."
        break
    fi
    sleep 2
    echo "  retry $i..."
done

# -------------------------------------------------------------------------------
# Stage 2: Launch Clio over Tor
# -------------------------------------------------------------------------------
CLIO_BIN="/usr/local/bin/clio_server"
CLIO_CFG="/opt/clio/etc/config.json"
CLIO_LOG="/var/log/clio.log"

echo "[+] Ensuring tor for Clio..."
if ! pgrep -f tor >/dev/null; then
    gosu debian-tor tor --RunAsDaemon 1
    sleep 5
fi

if [ -x "$CLIO_BIN" ]; then
    echo "[+] Starting Clio via torsocks..."
    unset HTTP_PROXY HTTPS_PROXY ALL_PROXY
    torsocks "$CLIO_BIN" --config "$CLIO_CFG" > "$CLIO_LOG" 2>&1 &
    sleep 5
    if ! pgrep -f clio_server >/dev/null; then
        echo "[!] Clio failed to start."
        cat "$CLIO_LOG" || true
        echo "[!] Container remains up; tailing log."
        exec tail -F "$CLIO_LOG"
    else
        echo "[✓] Clio is running."
        #set below on a debug switch to keep container logs cleaner
        exec tail -F "$CLIO_LOG"
    fi
else
    echo "[!] Clio not found; container idle."
    exec tail -f /dev/null
fi
