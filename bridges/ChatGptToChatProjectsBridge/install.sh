#!/usr/bin/env bash
#
# install.sh — ChatGptToChatProjectsBridge
#
# - Creates Python venv and installs dependencies
# - Installs and enables systemd user units
# - Registers drop-box profiles with ThePostalService
#

set -euo pipefail

BRIDGE_ROOT="$HOME/Code/ChatGptToChatProjectsBridge"
SYSTEMD_USER_DIR="$HOME/.config/systemd/user"

echo "[ChatGptToChatProjectsBridge] Starting installation..."

# Python venv
cd "$BRIDGE_ROOT"
if [[ ! -d "venv" ]]; then
    echo "[ChatGptToChatProjectsBridge] Creating virtual environment..."
    python3 -m venv venv
fi

echo "[ChatGptToChatProjectsBridge] Installing Python dependencies..."
source venv/bin/activate
pip install --quiet curl_cffi python-dotenv pika browser-cookie3 keyring

# Systemd units
echo "[ChatGptToChatProjectsBridge] Installing systemd user units..."
mkdir -p "$SYSTEMD_USER_DIR"

ln -sf \
    "$BRIDGE_ROOT/systemd/chatgptprojects.service" \
    "$SYSTEMD_USER_DIR/chatgptprojects.service"

ln -sf \
    "$BRIDGE_ROOT/systemd/chatgptprojects.timer" \
    "$SYSTEMD_USER_DIR/chatgptprojects.timer"

systemctl --user daemon-reload
systemctl --user enable --now chatgptprojects.timer

# Postal Service registration
echo "[ChatGptToChatProjectsBridge] Registering with The Postal Service..."
./register.sh

echo ""
echo "[ChatGptToChatProjectsBridge] Installation complete."
echo ""
echo "Next steps:"
echo "  1. Make sure you are logged into chatgpt.com in Chrome — no other setup needed."
echo "     The script reads your session cookie directly from Chrome's cookie store."
echo "     A stable device ID will be generated automatically on first run."
echo ""
echo "  2. Run once to test:"
echo "     cd $BRIDGE_ROOT && source venv/bin/activate && python3 bin/extract_projects.py"
