#!/usr/bin/env bash
#
# register.sh — ChatGptToChatProjectsBridge
#
# Drops two profiles into ThePostalService registry:
#   1. This service (sender)
#   2. chat-projects (recipient) — so ThePostalService knows where to deliver
#

set -euo pipefail

BRIDGE_ROOT="$HOME/Code/ChatGptToChatProjectsBridge"
REGISTRY_DIR="$HOME/Code/ThePostalService/registry"

echo "[ChatGptToChatProjectsBridge] Registering with The Postal Service..."

mkdir -p "$REGISTRY_DIR"

# Sender profile
cat <<EOF > "$REGISTRY_DIR/chatgpt-projects-bridge.toml"
service_name = "chatgpt-projects-bridge"
outbox_path = "$BRIDGE_ROOT/outbox"
inbox_path = "$BRIDGE_ROOT/inbox"
EOF

# Recipient profile (ChatProjects inbox)
cat <<EOF > "$REGISTRY_DIR/chat-projects.toml"
service_name = "chat-projects"
outbox_path = "$HOME/Code/ChatProjects/outbox"
inbox_path = "$HOME/Code/ChatProjects/inbox"
EOF

echo "[ChatGptToChatProjectsBridge] Registered:"
echo "  Sender:    chatgpt-projects-bridge → $BRIDGE_ROOT/outbox"
echo "  Recipient: chat-projects           → $HOME/Code/ChatProjects/inbox"
