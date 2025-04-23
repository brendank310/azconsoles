#!/bin/bash

set -euo pipefail

# Configuration
SERIAL_DEV="/dev/pts/${1}"
PPP_PEER_FILE="/etc/ppp/peers/pts${1}"
LOCAL_IP="10.0.0.2"
REMOTE_IP="10.0.0.1"

# Install required packages
echo "[*] Installing PPP and SSH..."
sudo apt-get update
sudo apt-get install -y ppp openssh-client

# Create PPP config
echo "[*] Writing PPP peer config to $PPP_PEER_FILE"
sudo tee "$PPP_PEER_FILE" >/dev/null <<EOF
$SERIAL_DEV 115200
noauth
local
nodetach
persist
$LOCAL_IP:$REMOTE_IP
lock
EOF

# Enable IP forwarding (if needed for routing)
sudo sysctl -w net.ipv4.ip_forward=1

# Start PPP connection
echo "[*] Starting PPP link on $SERIAL_DEV..."
sudo pppd call pts${1} &

# Wait for ppp0 to come up
echo "[*] Waiting for ppp0 interface..."
while ! ip addr show ppp0 &>/dev/null; do sleep 1; done
echo "[*] ppp0 is up!"
echo "ssh -i youprivkey.pem azureuser@10.0.0.1"

