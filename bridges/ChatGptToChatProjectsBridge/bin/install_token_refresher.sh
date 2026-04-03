#!/bin/bash
# install_token_refresher.sh
# One-time setup for ChatProjects token refresher automation

set -e

BRIDGE_DIR="/home/craigpar/Code/ChatGptToChatProjectsBridge"
SYSTEMD_SERVICE="$BRIDGE_DIR/systemd/chatprojects-token-api.service"
USER="craigpar"

echo "🔧 ChatProjects Token Refresher - Automated Setup"
echo "=================================================="
echo ""

# Step 1: Install systemd service
echo "📦 Step 1: Installing systemd service..."
if [ ! -d ~/.config/systemd/user ]; then
    mkdir -p ~/.config/systemd/user
fi

cp "$SYSTEMD_SERVICE" ~/.config/systemd/user/chatprojects-token-api.service
systemctl --user daemon-reload
systemctl --user enable chatprojects-token-api.service
systemctl --user start chatprojects-token-api.service

echo "✅ Systemd service installed and started"
echo ""

# Step 2: Verify service is running
echo "📋 Step 2: Checking service status..."
if systemctl --user is-active --quiet chatprojects-token-api.service; then
    echo "✅ Service is running on http://localhost:8000"
else
    echo "⚠️  Service failed to start. Check logs: journalctl --user -u chatprojects-token-api.service"
    exit 1
fi
echo ""

# Step 3: Instructions for Chrome extension
echo "🔌 Step 3: Install Chrome Extension (One-Time Manual Step)"
echo "============================================================"
echo ""
echo "1. Open Chrome and go to: chrome://extensions/"
echo "2. Enable 'Developer mode' (toggle in top-right corner)"
echo "3. Click 'Load unpacked'"
echo "4. Select folder: $BRIDGE_DIR/chrome_extension/"
echo ""
echo "5. Verify it's working:"
echo "   - Extension should appear in Chrome toolbar"
echo "   - Open Chrome DevTools (Cmd+Option+J or Ctrl+Shift+J)"
echo "   - You should see logs: '[ChatProjects Token Refresher] Token updated successfully'"
echo ""

# Step 4: Final check
echo "📝 Step 4: Your Cron is Already Set Up"
echo "======================================"
echo ""
echo "Your existing cron job will now automatically:"
echo "  1. Use the fresh token from .env"
echo "  2. Sync conversations from ChatGPT"
echo "  3. Import them into ChatProjects"
echo "  4. Trigger Lean Inception generation"
echo ""
echo "Cron schedule: Every 7 minutes (and other times)"
echo ""

# Step 5: Show logs
echo "📊 Service Logs"
echo "==============="
echo "To monitor the token API service:"
echo "  journalctl --user -u chatprojects-token-api.service -f"
echo ""
echo "To check system cron logs:"
echo "  tail -f /home/craigpar/.cache/chatprojects-schedule.log"
echo "  tail -f /home/craigpar/.cache/chatprojects-queue.log"
echo ""

echo "🎉 Setup complete! Everything is now automatic."
echo ""
