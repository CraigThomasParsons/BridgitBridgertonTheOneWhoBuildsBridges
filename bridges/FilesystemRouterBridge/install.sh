#!/usr/bin/env bash
# ============================================================================
# FilesystemRouterBridge Installer
#
# Sets up the filesystem router as a systemd timer service.
# This enables automatic routing of packages between bridges every 30 seconds.
#
# Requires: systemd, bash
# Usage: ./install.sh
# ============================================================================

set -euo pipefail

BRIDGE_ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "Installing FilesystemRouterBridge..."

# Make the routing script executable.
chmod +x "$BRIDGE_ROOT/bin/route.sh"

# Copy systemd unit files to the user systemd directory.
# User-level services don't require root and restart on login.
mkdir -p ~/.config/systemd/user

cp "$BRIDGE_ROOT/systemd/filesystem-router.service" ~/.config/systemd/user/
cp "$BRIDGE_ROOT/systemd/filesystem-router.timer" ~/.config/systemd/user/

# Reload systemd to pick up the new units.
systemctl --user daemon-reload

# Enable the timer to start on boot.
systemctl --user enable filesystem-router.timer

# Start the timer immediately.
systemctl --user start filesystem-router.timer

echo "FilesystemRouterBridge installed and timer started."
echo "Check status: systemctl --user status filesystem-router.timer"
