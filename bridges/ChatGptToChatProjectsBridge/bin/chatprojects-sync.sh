#!/usr/bin/env bash
set -euo pipefail

SOURCE_ROOT="/home/craigpar/Code/ChatProjects"
TARGET_ROOT="/home/craigpar/Projects"
PROJECTS_SOURCE="/home/craigpar/Documents/Projects"
PROJECTS_TARGET="$TARGET_ROOT/Projects"
LOG_PREFIX="[chatprojects-sync]"
STATUS_FILE="/home/craigpar/.cache/chatprojects-sync.status"

on_error() {
  local exit_code=$?
  printf 'status=error\nexit_code=%s\nlast_run=%s\n' "$exit_code" "$(date -Iseconds)" > "$STATUS_FILE"
  echo "$LOG_PREFIX $(date -Iseconds) sync failed (exit=$exit_code)"
}

trap on_error ERR

mkdir -p /home/craigpar/.cache

echo "$LOG_PREFIX $(date -Iseconds) starting sync"

# Main mirror. Exclude runtime state directories that are often docker-owned
# and cause cron rsync failures for non-root users.
/usr/bin/rsync -az --delete --copy-dirlinks --omit-dir-times \
  --no-perms --no-owner --no-group \
  --exclude='.env' \
  --exclude='.env.*' \
  --exclude='storage/***' \
  --exclude='bootstrap/cache/***' \
  --exclude-from="$SOURCE_ROOT/.gitignore" \
  "$SOURCE_ROOT/" "$TARGET_ROOT/"

# Ensure projects are materialized as real directories in TARGET_ROOT.
if [[ -L "$PROJECTS_TARGET" ]]; then
  rm -f "$PROJECTS_TARGET"
fi
mkdir -p "$PROJECTS_TARGET"

/usr/bin/rsync -az --delete --omit-dir-times \
  --no-perms --no-owner --no-group \
  "$PROJECTS_SOURCE/" "$PROJECTS_TARGET/"

# Keep expected writable paths available for the Laravel app.
mkdir -p "$TARGET_ROOT/storage/logs" "$TARGET_ROOT/bootstrap/cache"
chmod -R ug+rwX "$TARGET_ROOT/storage" "$TARGET_ROOT/bootstrap/cache" 2>/dev/null || true

printf 'status=ok\nexit_code=0\nlast_run=%s\n' "$(date -Iseconds)" > "$STATUS_FILE"

echo "$LOG_PREFIX $(date -Iseconds) sync complete"