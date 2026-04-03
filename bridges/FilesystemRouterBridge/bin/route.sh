#!/usr/bin/env bash
# ============================================================================
# FilesystemRouterBridge — Route packages between bridge outboxes and inboxes
#
# This script reads route definitions from registry/routes.toml and moves
# packages from source outbox directories to destination inbox directories.
# It is the connective tissue between all Bridgit bridges, implementing the
# core "files over abstractions" philosophy.
#
# Each package is a directory containing at minimum a letter.toml file that
# declares the intended recipient. The router uses the route table to decide
# where to deliver each package.
#
# Usage:
#   ./bin/route.sh              # Run once (cron/timer mode)
#   ./bin/route.sh --watch      # Watch continuously (daemon mode)
#
# Designed for systemd timer or continuous daemon operation.
# ============================================================================

set -euo pipefail

# Resolve the bridge root directory relative to this script.
# This allows the script to work regardless of the current working directory.
BRIDGE_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Resolve the project root (two levels up from the bridge).
# Routes reference paths relative to the project root.
PROJECT_ROOT="$(cd "$BRIDGE_ROOT/../.." && pwd)"

# Log directory for operational visibility.
LOG_DIR="$BRIDGE_ROOT/logs"
mkdir -p "$LOG_DIR"

# Timestamp for log entries — ISO 8601 format for consistent parsing.
log() {
    echo "[$(date -Iseconds)] $*" | tee -a "$LOG_DIR/router.log"
}

# ---------------------------------------------------------------------------
# Route a single package from source to destination
# ---------------------------------------------------------------------------
route_package() {
    local package_dir="$1"
    local destination_dir="$2"
    local package_name
    package_name="$(basename "$package_dir")"

    # Create the destination directory if it doesn't exist.
    # This handles first-run cases where inbox directories are fresh.
    mkdir -p "$destination_dir"

    local target="$destination_dir/$package_name"

    # Check for collision — don't overwrite packages already in the destination.
    # Collisions indicate a processing bottleneck or duplicate deliveries.
    if [ -d "$target" ]; then
        log "SKIP: $package_name already exists at $target"
        return 0
    fi

    # Move the package atomically. Using mv ensures no partial copies.
    # This is only safe within the same filesystem — cross-device moves
    # fall back to cp+rm automatically via mv.
    mv "$package_dir" "$target"
    log "ROUTED: $package_name → $destination_dir"
}

# ---------------------------------------------------------------------------
# Process all routes defined in the registry
# ---------------------------------------------------------------------------
process_routes() {
    local routes_file="$BRIDGE_ROOT/registry/routes.toml"

    # Validate that the routes file exists before processing.
    if [ ! -f "$routes_file" ]; then
        log "ERROR: Routes file not found at $routes_file"
        return 1
    fi

    # Parse TOML routes using simple grep/sed extraction.
    # Each route block has: name, source, destination, pattern.
    # We extract source and destination pairs for routing.
    local route_count=0

    # Extract route blocks by finding source/destination pairs.
    # This is a minimal TOML parser that handles our specific format.
    local sources destinations
    sources=$(grep '^source' "$routes_file" | sed 's/source = "//;s/"//')
    destinations=$(grep '^destination' "$routes_file" | sed 's/destination = "//;s/"//')

    # Combine sources and destinations into parallel arrays.
    # Using paste to pair them line-by-line for iteration.
    paste <(echo "$sources") <(echo "$destinations") | while IFS=$'\t' read -r source destination; do
        local full_source="$PROJECT_ROOT/$source"
        local full_destination="$PROJECT_ROOT/$destination"

        # Skip routes where the source directory doesn't exist yet.
        # The upstream bridge may not have produced output yet.
        if [ ! -d "$full_source" ]; then
            continue
        fi

        # Route each package (subdirectory) in the source outbox.
        for package_dir in "$full_source"/*/; do
            # Glob expands to literal "/*/" if no matches — skip that case.
            [ -d "$package_dir" ] || continue

            route_package "$package_dir" "$full_destination"
            route_count=$((route_count + 1))
        done
    done

    log "Routing complete. Processed $route_count package(s)."
}

# ---------------------------------------------------------------------------
# Main entry point
# ---------------------------------------------------------------------------

log "FilesystemRouterBridge starting..."

if [ "${1:-}" = "--watch" ]; then
    # Continuous watch mode — poll every 5 seconds.
    # This is the daemon mode for systemd service operation.
    log "Running in watch mode (polling every 5s)..."
    while true; do
        process_routes
        sleep 5
    done
else
    # Single-run mode — process once and exit.
    # This is the cron/timer mode for periodic execution.
    process_routes
fi
