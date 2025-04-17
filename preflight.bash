#!/bin/bash
set -euo pipefail
DEBUG_BUILD=false
VERBOSE_DEBUG=false
PURGE_MODE=false
UNINSTALL_MODE=false
REBUILD_MODE=false
HOST_SPOOLER=false
for arg in "$@"; do
    case "$arg" in
        --debug) DEBUG_BUILD=true ;;
        --verbose) VERBOSE_DEBUG=true ;;
        --purge) PURGE_MODE=true ;;
        --uninstall) UNINSTALL_MODE=true ;;
        --rebuild) REBUILD_MODE=true ;;
        --cold-start) COLD_START="--cold-start" ;;
        --host-spooler) HOST_SPOOLER=true ;;
    esac
done
: "${COLD_START:=}"
log_debug() {
    if [ "$VERBOSE_DEBUG" = true ]; then echo "[debug] $1"; fi
}
SCRIPT_PATH="$(readlink -f "$0")"
PROJECT_ROOT="$(dirname "$SCRIPT_PATH")"
MODULES_DIR="$PROJECT_ROOT/modules"
GCC_PKG_DIR="$PROJECT_ROOT/gcc-packages/archives"
TRINITY_CPP="$PROJECT_ROOT/trinity.cpp"
TRINITY_OUT="$PROJECT_ROOT/gcc-packages/trinity"
TRINITY_LOG="$PROJECT_ROOT/gcc-packages/trinity.log"
CLIO_URL="https://github.com/XRPLF/clio/releases/download/2.4.0/clio_server_Linux_Release_gcc_2.4.0.zip"
CLIO_ZIP="$PROJECT_ROOT/gcc-packages/clio.zip"
CLIO_BIN="$PROJECT_ROOT/gcc-packages/clio_server"
COMPOSE_FILE="$MODULES_DIR/docker-compose.yml"
TRINITY_SOCK_HOST="/var/run/trinity-host.sock"
sudo mkdir -p "$(dirname "$TRINITY_LOG")"
sudo chmod 777 "$(dirname "$TRINITY_LOG")"
kill_trinity() {
    local pids
    pids=$(pgrep -f "$TRINITY_OUT" || true)
    if [ -n "$pids" ]; then
        sudo kill -9 $pids
        sleep 1
    fi
}
if [ "$UNINSTALL_MODE" = true ]; then
    kill_trinity
    sudo rm -f "$TRINITY_SOCK_HOST"
    docker compose -f "$COMPOSE_FILE" down || true
    docker ps -aq | xargs -r docker rm -f
    docker volume ls -q | xargs -r docker volume rm
    docker network ls --format '{{.Name}}' | grep -vE '^(bridge|host|none)$' | xargs -r docker network rm
    docker images --format '{{.Repository}}:{{.Tag}}' | grep -E 'cloudstorm|ipfs|symbiote' | xargs -r docker rmi -f || true
    docker system prune -a --volumes -f
    rm -rf "$PROJECT_ROOT/gcc-packages" "$PROJECT_ROOT/trinity-host" "$MODULES_DIR"/*/gcc-packages
    echo "[✓] CloudStorm uninstalled."
    exit 0
fi
if [ "$PURGE_MODE" = true ]; then
    docker system prune -a --volumes -f
    exit 0
fi
if [ "$REBUILD_MODE" = false ]; then
    apt-get update
    apt-get install -y build-essential g++ libboost-all-dev libssl-dev ca-certificates curl \
        gnupg lsb-release apt-utils unzip wget jq git make libgmp-dev libmpfr-dev libmpc-dev \
        flex bison xz-utils software-properties-common iproute2 yq
    apt-get autoremove -y
    apt-get clean
    rm -rf /var/lib/apt/lists/*
    if ! command -v docker >/dev/null 2>&1; then
        echo "[!] Docker not found."
        exit 1
    fi
    usermod -aG docker "${SUDO_USER:-$(whoami)}" || true
fi
mkdir -p "$GCC_PKG_DIR"
REQUIRED_PACKAGES=(gcc-13 g++-13 cpp-13 gcc-13-base g++-13-x86-64-linux-gnu gcc-13-x86-64-linux-gnu cpp-13-x86-64-linux-gnu
    libstdc++6 libstdc++-13-dev libgcc-13-dev libasan6 libasan8 libubsan1 liblsan0 libatomic1
    libgomp1 libquadmath0 libhwasan0 libitm1 libcc1-0 libbinutils libctf0 libctf-nobfd0 libgprofng0
    libsframe1 libisl23 libmpc3 libmpfr6 binutils binutils-common binutils-x86-64-linux-gnu
    gcc-11-base gcc-14-base libc6-dev libjansson4)
if [ "$REBUILD_MODE" = false ] || ! ls "$GCC_PKG_DIR"/*.deb >/dev/null 2>&1; then
    apt-get update
    for pkg in "${REQUIRED_PACKAGES[@]}"; do
        if ! ls "$GCC_PKG_DIR"/"$pkg"_*.deb >/dev/null 2>&1; then
            apt-get download "$pkg"
            mv -f "$pkg"_*.deb "$GCC_PKG_DIR/" 2>/dev/null || true
        fi
    done
    apt-get install -y --allow-downgrades "$GCC_PKG_DIR"/*.deb
fi
SERVICE_COUNT=$(yq '.services | length' "$COMPOSE_FILE")
EXPECTED_PEER_COUNT=$((SERVICE_COUNT + 1))
export EXPECTED_PEER_COUNT
export TRINITY_SOCK_PATH="$TRINITY_SOCK_HOST"
sudo rm -f "$TRINITY_SOCK_HOST"
HARDCODED_TAG=$(grep -Po 'static\s+const\s+std::string\s+HOST_PEERNAME\s*=\s*"\K[^"]+' "$TRINITY_CPP" || true)
: "${HARDCODED_TAG:=genesis}"

if $DEBUG_BUILD; then
    g++ -std=c++11 -g -O0 "$TRINITY_CPP" -o "$TRINITY_OUT" \
      -Wl,-Bstatic -lboost_system -lboost_filesystem \
      -Wl,-Bdynamic -lssl -lcrypto -lpthread \
      -static-libstdc++ -static-libgcc
else
    g++ -std=c++11 -O2 "$TRINITY_CPP" -o "$TRINITY_OUT" \
      -Wl,-Bstatic -lboost_system -lboost_filesystem \
      -Wl,-Bdynamic -lssl -lcrypto -lpthread \
      -static-libstdc++ -static-libgcc
fi


if [ ! -x "$TRINITY_OUT" ]; then
    echo "[!] Trinity build failed."
    exit 1
fi
if [ "$REBUILD_MODE" = false ] || [ ! -x "$CLIO_BIN" ]; then
    mkdir -p "$(dirname "$CLIO_ZIP")"
    rm -f "$CLIO_ZIP" "$CLIO_BIN"
    curl -L "$CLIO_URL" -o "$CLIO_ZIP"
    unzip -j "$CLIO_ZIP" -d "$(dirname "$CLIO_BIN")"
    chmod +x "$CLIO_BIN"
fi
echo "[+] Syncing Docker contexts..."
for service in $(yq -r '.services | keys[]' "$COMPOSE_FILE"); do
    context=$(yq -r ".services.\"$service\".build.context // .services.\"$service\".build" "$COMPOSE_FILE")
    [ -z "$context" ] && echo "[!] No context found for $service" && continue
    mod_dir="$MODULES_DIR/$context"
    MODULE_NAME="$(basename "$mod_dir")"
    SOCKET_PATH="/var/run/trinity-${MODULE_NAME}.sock"
    mkdir -p "$mod_dir/gcc-packages/archives"
    rm -f "$mod_dir/gcc-packages/archives"/*.deb
    cp -f "$GCC_PKG_DIR"/*.deb "$mod_dir/gcc-packages/archives/"
    mkdir -p "$mod_dir/gcc-packages"
    cp -f "$TRINITY_OUT" "$mod_dir/gcc-packages/trinity"
    echo "$SOCKET_PATH" > "$mod_dir/TRINITY_SOCK_PATH"
    echo "$TRINITY_SOCK_HOST" > "$mod_dir/TRINITY_SOCK_PATH_HOST"
    if [[ "$MODULE_NAME" == "symbiote" ]]; then
        [ -x "$CLIO_BIN" ] && cp -f "$CLIO_BIN" "$mod_dir/gcc-packages/clio_server"
    else
        rm -f "$mod_dir/gcc-packages/clio_server" || true
    fi
    echo "  → Updated $MODULE_NAME:"
    echo "     - Context:          $context"
    echo "     - Container socket: $SOCKET_PATH"
    echo "     - Host socket:      $TRINITY_SOCK_HOST"
done
echo "[+] Building and launching CloudStorm..."
cd "$MODULES_DIR"
docker compose build --no-cache
docker compose up -d
echo "[+] Cleaning up build artifacts..."
for mod in "$MODULES_DIR"/*; do
    [ -d "$mod" ] && rm -rf "$mod/gcc-packages"
done
echo "[✓] CloudStorm bootstrap complete."
