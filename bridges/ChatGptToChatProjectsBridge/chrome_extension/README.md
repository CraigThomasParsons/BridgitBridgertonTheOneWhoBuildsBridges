# ChatProjects Token Refresher - Chrome Extension

Automatically refreshes your ChatGPT session token and updates the bridge `.env` file, ensuring conversations sync reliably without any manual intervention.

## ✨ How It Works (Fully Automatic)

1. **Systemd service** starts on boot and runs the Token API server on `localhost:8000`
2. **Chrome extension** (running in your browser) extracts session token every 1 minute
3. **Extension POSTs token** to the local API server
4. **API server updates** `.env` with fresh token
5. **Cron runs** conversation sync - always uses valid token
6. **Conversations imported** → Lean Inceptions auto-generated

## 🚀 One-Time Setup

Run this once:

```bash
bash /home/craigpar/Code/ChatGptToChatProjectsBridge/bin/install_token_refresher.sh
```

This will:
- ✅ Install and enable the systemd service (runs automatically on boot)
- ✅ Start the Token API server immediately
- ✅ Show you how to install the Chrome extension (the only manual step)

## 🔌 Install Chrome Extension (One-Time Manual)

After running the install script:

1. Open Chrome → `chrome://extensions/`
2. Enable **Developer mode** (top-right toggle)
3. Click **Load unpacked**
4. Select: `/home/craigpar/Code/ChatGptToChatProjectsBridge/chrome_extension/`
5. **Stay logged into ChatGPT** in your browser

That's it! The extension will auto-refresh your token every minute.

## ✅ Verify It's Working

1. **Check service is running:**
   ```bash
   systemctl --user status chatprojects-token-api.service
   ```

2. **Check service logs:**
   ```bash
   journalctl --user -u chatprojects-token-api.service -f
   ```

3. **Check Chrome extension logs:**
   - Open DevTools: `Cmd+Option+J` (Mac) or `Ctrl+Shift+J` (Windows/Linux)
   - Watch for: `[ChatProjects Token Refresher] Token updated successfully`

4. **Check .env was updated:**
   ```bash
   grep CHATGPT_SESSION_TOKEN /home/craigpar/Code/ChatGptToChatProjectsBridge/.env
   ```

## 📁 Files

- **manifest.json** - Extension config (Manifest V3)
- **background.js** - Service worker (extracts token every minute)
- **content.js** - Content script (minimal, for debugging)
- **../bin/token_api_server.py** - API server (runs via systemd)
- **../systemd/chatprojects-token-api.service** - Systemd service file
- **../bin/install_token_refresher.sh** - One-time setup script

## 🛠️ Troubleshooting

### "Token not found" errors
- Ensure you're **logged into ChatGPT** in at least one Chrome tab
- Token only refreshes if you have an active ChatGPT session

### Service won't start
```bash
journalctl --user -u chatprojects-token-api.service
```
Check for permission errors or port conflicts (8000 in use?)

### Extension not updating token
- Check browser DevTools console for errors
- Verify `.env` file is readable/writable
- Try disabling/re-enabling extension in `chrome://extensions/`

### "Connection refused" to localhost:8000
- Service might not be running. Check: `systemctl --user status chatprojects-token-api.service`
- Restart service: `systemctl --user restart chatprojects-token-api.service`

## 🔄 Remove/Uninstall

To disable token refreshing:

```bash
systemctl --user disable chatprojects-token-api.service
systemctl --user stop chatprojects-token-api.service
# Remove extension from chrome://extensions/
```

## ✨ After Setup - Fully Automatic Flow

Once installed, **no more manual work needed**:

```
Every 7 minutes (cron):
  ├─ Cron reads fresh CHATGPT_SESSION_TOKEN from .env (kept fresh by extension)
  ├─ Calls: php artisan conversations:sync-remote
  ├─ Python script syncs conversations from ChatGPT
  ├─ Imports into database
  ├─ Fires ConversationImported event
  └─ Queue processes Lean Inception generation via Claude

Result: Your ChatProjects keeps up with ChatGPT conversations automatically 🎉
```

