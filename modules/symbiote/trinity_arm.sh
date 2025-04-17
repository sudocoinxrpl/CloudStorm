#!/bin/bash
set -euo pipefail

# Common Trinity bootstrap script (trinity_arm.sh)
# This script is universal across modules. It launches the Trinity daemon,
# waits for full readiness via its UNIX domain socket, and exits with success
# only when Trinity reports full readiness.

ulimit -c unlimited
if [ -w /proc/sys/kernel/core_pattern ]; then
    echo "/tmp/core.%e.%p.%h.%t" > /proc/sys/kernel/core_pattern
fi

# Files where preflight writes the socket paths
CONTAINER_SOCK_FILE="/opt/CloudStorm/TRINITY_SOCK_PATH"
HOST_SOCK_FILE="/opt/CloudStorm/TRINITY_SOCK_PATH_HOST"

# Read Trinity socket path from file or exit if not found
if [ -f "$CONTAINER_SOCK_FILE" ]; then
    TRINITY_SOCK="$(tr -d '\r\n' < "$CONTAINER_SOCK_FILE")"
else    
    echo "[trinity_arm.sh] No TRINITY_SOCK_PATH file found"
    exit 1
fi

export TRINITY_SOCK_PATH="$TRINITY_SOCK"

# Read host Trinity socket path from file or default to a known value
if [ -f "$HOST_SOCK_FILE" ]; then
    HOST_TRINITY_SOCK="$(tr -d '\r\n' < "$HOST_SOCK_FILE")"
else
    HOST_TRINITY_SOCK="/var/run/trinity-host.sock"
fi

NODE_ID="${HOSTNAME:-$(hostname)}"
TRINITY_EXEC="/opt/CloudStorm/CloudStorm/trinity"
WORKDIR="/opt/CloudStorm"
TRINITY_RAM="/dev/shm/trinity"
MAX_ATTEMPTS=45
RE_SIGNAL_EVERY=10

echo "[trinity_arm.sh] Using container Trinity socket: $TRINITY_SOCK"
echo "[trinity_arm.sh] Node ID: $NODE_ID"

# -------------------------------------------------------------------------------
# Launch Trinity
# -------------------------------------------------------------------------------
echo "[trinity_arm.sh] Launching Trinity..."
nohup "$TRINITY_EXEC" "$WORKDIR" > /tmp/trinity.log 2>&1 &
sleep 2
if ! pgrep -f "$TRINITY_EXEC" >/dev/null; then
    echo "[trinity_arm.sh] Trinity startup failed."
    cat /tmp/trinity.log || echo "[trinity_arm.sh] No Trinity log available."
    exit 1
fi

# -------------------------------------------------------------------------------
# Stage A: Wait for Trinity readiness
# -------------------------------------------------------------------------------
echo "[trinity_arm.sh] Waiting for Trinity to report full readiness..."
attempt=0
while [ $attempt -lt $MAX_ATTEMPTS ]; do
    readyResp=$(curl --unix-socket "$TRINITY_SOCK" -s --max-time 3 http://localhost/ready || echo "{}")
    ready=$(echo "$readyResp" | jq -r '.ready' 2>/dev/null || echo "false")
    consensusResp=$(curl --unix-socket "$TRINITY_SOCK" -s --max-time 3 http://localhost/consensus || echo "{}")
    cert=$(echo "$consensusResp" | jq -r '.cert' 2>/dev/null || echo "")
    key=$(echo "$consensusResp" | jq -r '.key' 2>/dev/null || echo "")

    echo "[trinity_arm.sh] Attempt $((attempt+1)): /ready=$ready, cert_length=${#cert}, key_length=${#key}"

    if [ "$ready" == "true" ] && [ ${#cert} -gt 0 ] && [ ${#key} -gt 0 ]; then
        echo "[trinity_arm.sh] Trinity reports full readiness."
        exit 0
    fi

    if (( (attempt+1) % RE_SIGNAL_EVERY == 0 )); then
        echo "[trinity_arm.sh] Re-signaling readiness..."
        curl --unix-socket "$TRINITY_SOCK" -s -X POST \
             -H "X-Node-ID: $NODE_ID" \
             http://localhost/tunnel/ready >/dev/null && echo "[trinity_arm.sh] Re-signal sent."
    fi

    sleep 2
    ((attempt++))
done

echo "[trinity_arm.sh] Timeout waiting for Trinity. Exiting..."
exit 1
