#!/bin/bash
set -euo pipefail

# Where the CloudStorm Go code lives

MODULE_ROOT="/opt/CloudStorm/CloudStorm"
BIN_OUTPUT="/opt/CloudStorm/cloudstorm"

echo "[module_bootstrap.sh] Checking for Go toolchain..."
if ! command -v go >/dev/null 2>&1; then
    echo "[module_bootstrap.sh] Go not found. Container will idle."
    exec tail -f /tmp/trinity.log
fi

cd "$MODULE_ROOT"

# Initialize go.mod
if [ ! -f go.mod ]; then
    echo "[module_bootstrap.sh] Initializing go.mod..."
    go mod init CloudStorm
fi

echo "[module_bootstrap.sh] Tidying modules..."
go mod tidy

echo "[module_bootstrap.sh] Building CloudStorm Go binary..."
if ! go build -o "$BIN_OUTPUT" main.go; then
    echo "[module_bootstrap.sh] Go build failed."
    cat /tmp/trinity.log || echo "[module_bootstrap.sh] Trinity log unavailable."
    exit 1
fi

echo "[module_bootstrap.sh] Launching CloudStorm..."
nohup "$BIN_OUTPUT" > /tmp/go_server.log 2>&1 &
sleep 4

if ! pgrep -f "$BIN_OUTPUT" >/dev/null; then
    echo "[module_bootstrap.sh] CloudStorm failed to launch."
    cat /tmp/go_server.log || echo "[module_bootstrap.sh] No Go log available."
    exit 1
else
    echo "[module_bootstrap.sh] CloudStorm is active."
fi

echo "[module_bootstrap.sh] Module bootstrap complete. Container now idling."
exec tail -f /tmp/go_server.log
