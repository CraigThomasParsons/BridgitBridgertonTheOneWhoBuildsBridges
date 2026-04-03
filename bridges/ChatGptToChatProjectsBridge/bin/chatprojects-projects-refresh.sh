#!/usr/bin/env bash
set -euo pipefail

BRIDGE_ROOT="/home/craigpar/Code/ChatGptToChatProjectsBridge"
CHATPROJECTS_ROOT="/home/craigpar/Code/ChatProjects"
PYTHON_BIN="$BRIDGE_ROOT/venv/bin/python3"
EXTRACT_SCRIPT="$BRIDGE_ROOT/bin/extract_projects.py"
IMPORT_CMD=(/usr/bin/php "$CHATPROJECTS_ROOT/artisan" conversations:import-local "$CHATPROJECTS_ROOT/inbox" --delete)
LOCK_FILE="/home/craigpar/.cache/chatprojects-projects-refresh.lock"
STATUS_FILE="/home/craigpar/.cache/chatprojects-projects-refresh.status"
LOG_PREFIX="[chatprojects-projects-refresh]"

mkdir -p /home/craigpar/.cache

on_error() {
  local exit_code=$?
  printf 'status=error\nexit_code=%s\nlast_run=%s\n' "$exit_code" "$(date -Iseconds)" > "$STATUS_FILE"
  echo "$LOG_PREFIX $(date -Iseconds) failed (exit=$exit_code)"
}
trap on_error ERR

exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  echo "$LOG_PREFIX $(date -Iseconds) skipped (another run in progress)"
  exit 0
fi

echo "$LOG_PREFIX $(date -Iseconds) starting extractor"
"$PYTHON_BIN" "$EXTRACT_SCRIPT"

echo "$LOG_PREFIX $(date -Iseconds) starting importer"
"${IMPORT_CMD[@]}"

printf 'status=ok\nexit_code=0\nlast_run=%s\n' "$(date -Iseconds)" > "$STATUS_FILE"
echo "$LOG_PREFIX $(date -Iseconds) complete"