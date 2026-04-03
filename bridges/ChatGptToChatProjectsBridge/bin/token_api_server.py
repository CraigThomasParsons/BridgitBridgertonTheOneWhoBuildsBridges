#!/usr/bin/env python3
"""
Token Update API Server
Simple HTTP API that receives ChatGPT session tokens from the Chrome extension
and updates the .env file.

Usage:
    python3 token_api_server.py

The server listens on http://localhost:8000 and exposes:
    POST /api/token/update - Receives token and updates .env
"""

import json
import os
import sys
from pathlib import Path
from http.server import HTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse

# Directory paths - go up one level from bin/ to bridge root
BRIDGE_DIR = Path(__file__).parent.parent
ENV_FILE = BRIDGE_DIR / ".env"

class TokenUpdateHandler(BaseHTTPRequestHandler):
    """HTTP request handler for token updates"""

    def _send_cors_headers(self):
        """Send CORS headers to allow Chrome extension requests"""
        self.send_header('Access-Control-Allow-Origin', '*')
        self.send_header('Access-Control-Allow-Methods', 'POST, OPTIONS')
        self.send_header('Access-Control-Allow-Headers', 'Content-Type')

    def do_OPTIONS(self):
        """Handle CORS preflight requests"""
        self.send_response(200)
        self._send_cors_headers()
        self.end_headers()

    def do_POST(self):
        """Handle POST requests"""

        # Route: POST /api/token/update
        if self.path == "/api/token/update":
            try:
                # Read request body
                content_length = int(self.headers.get('Content-Length', 0))
                body = self.rfile.read(content_length).decode('utf-8')
                data = json.loads(body)

                token = data.get('token')
                device_id = data.get('device_id')

                if not token:
                    self.send_response(400)
                    self._send_cors_headers()
                    self.send_header('Content-Type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"error": "Missing token in request"}).encode())
                    return

                # Update .env file
                update_env_token(token, device_id)

                # Send success response
                self.send_response(200)
                self._send_cors_headers()
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                response = {
                    "message": "Token updated successfully",
                    "token_length": len(token),
                    "env_file": str(ENV_FILE)
                }
                self.wfile.write(json.dumps(response).encode())

                print(f"[Token API] Token updated successfully (length: {len(token)})")

            except json.JSONDecodeError:
                self.send_response(400)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"error": "Invalid JSON"}).encode())
            except Exception as e:
                print(f"[Token API] Error: {e}", file=sys.stderr)
                self.send_response(500)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"error": str(e)}).encode())
        else:
            self.send_response(404)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({"error": "Not found"}).encode())

    def log_message(self, format, *args):
        """Suppress default logging"""
        print(f"[Token API] {format % args}")


def update_env_token(token, device_id=None):
    """Update the CHATGPT_SESSION_TOKEN in .env file"""

    if not ENV_FILE.exists():
        raise FileNotFoundError(f".env file not found at {ENV_FILE}")

    # Read existing .env
    env_content = ENV_FILE.read_text()
    lines = env_content.split('\n')

    # Update or add the token line
    updated_lines = []
    token_found = False

    for line in lines:
        if line.strip().startswith('CHATGPT_SESSION_TOKEN='):
            updated_lines.append(f"CHATGPT_SESSION_TOKEN='{token}'")
            token_found = True
        else:
            updated_lines.append(line)

    # If token line didn't exist, add it
    if not token_found:
        updated_lines.insert(0, f"CHATGPT_SESSION_TOKEN='{token}'")

    # Write back to .env
    ENV_FILE.write_text('\n'.join(updated_lines))
    print(f"[Token API] .env updated at {ENV_FILE}")


def main():
    """Start the token update API server"""

    PORT = 8000
    server_address = ('', PORT)
    httpd = HTTPServer(server_address, TokenUpdateHandler)

    print(f"[Token API] Starting Token Update API Server on port {PORT}...")
    print(f"[Token API] Listening at http://localhost:{PORT}")
    print(f"[Token API] POST /api/token/update to update token")
    print(f"[Token API] .env file: {ENV_FILE}")
    print("")

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n[Token API] Server stopped.")
        httpd.server_close()


if __name__ == "__main__":
    main()
