#!/bin/bash
set -euo pipefail
# tor_installer.sh: A proper installer to configure Tor with a hidden service for onionscan.
# This script removes any existing hidden service data, writes a new torrc,
# sets secure permissions, verifies the configuration as debian-tor, and restarts Tor.
# Finally, it displays the generated onion hostname.
#
# Usage: Run as root (e.g., sudo ./tor_installer.sh)

# Check if running as root
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

# Hidden Service configuration for onionscan on port 1234
HiddenServiceDir /var/lib/tor/hidden_service_onionscan/
HiddenServicePort 1234 127.0.0.1:1234
EOF

echo "Ensuring hidden service directory exists and setting secure permissions..."
mkdir -p /var/lib/tor/hidden_service_onionscan/
chown -R debian-tor:debian-tor /var/lib/tor/hidden_service_onionscan/
chmod -R 700 /var/lib/tor/hidden_service_onionscan/

echo "Verifying Tor configuration as debian-tor..."
if ! sudo -u debian-tor tor --verify-config; then
  echo "Tor configuration verification failed. Please check your configuration and logs." >&2
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



