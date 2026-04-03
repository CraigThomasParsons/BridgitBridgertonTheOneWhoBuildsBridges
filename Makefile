# ============================================================================
# Bridgit Sync Engine — Root Makefile
#
# Provides build, test, and run targets for the entire workspace.
# The engine spans two Go modules (root and tools/scaffolder) plus
# a Rust bridge and several shell/Python bridges.
# ============================================================================

# Default shell with strict error handling.
SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

# Go settings shared across both modules.
GO        := go
GO_FLAGS  := -v
BUILD_OUT := bin

# ============================================================================
# Phony targets — none of these produce files with these names.
# ============================================================================
.PHONY: all build build-scaffolder test test-scaffolder run route \
        tidy clean help

# Default target — build everything.
all: build build-scaffolder

# ----------------------------------------------------------------------------
# Build targets
# ----------------------------------------------------------------------------

# build: Compile the main Bridgit sync engine binary.
# Output lands in ./bin/bridgit so runtime scripts can find it reliably.
build:
	@echo "==> Building bridgit engine..."
	$(GO) build $(GO_FLAGS) -o $(BUILD_OUT)/bridgit ./cmd/bridgit

# build-scaffolder: Compile the scaffolder tool from its own module.
# The scaffolder lives in a separate Go module under tools/scaffolder/.
build-scaffolder:
	@echo "==> Building scaffolder..."
	cd tools/scaffolder && $(GO) build $(GO_FLAGS) -o ../../$(BUILD_OUT)/scaffolder ./cmd/scaffolder

# ----------------------------------------------------------------------------
# Test targets
# ----------------------------------------------------------------------------

# test: Run all tests for the root module.
test:
	@echo "==> Testing bridgit engine..."
	$(GO) test ./...

# test-scaffolder: Run all tests for the scaffolder module.
test-scaffolder:
	@echo "==> Testing scaffolder..."
	cd tools/scaffolder && $(GO) test ./...

# test-all: Run tests across every Go module in the workspace.
test-all: test test-scaffolder

# ----------------------------------------------------------------------------
# Run targets
# ----------------------------------------------------------------------------

# run: Execute the sync engine directly without a separate build step.
# Useful during development — builds and runs in a single command.
run:
	@echo "==> Running bridgit engine..."
	$(GO) run ./cmd/bridgit

# route: Execute the FilesystemRouterBridge once.
# Routes any pending packages sitting in bridge outboxes.
route:
	@echo "==> Running filesystem router..."
	./bridges/FilesystemRouterBridge/bin/route.sh

# ----------------------------------------------------------------------------
# Maintenance targets
# ----------------------------------------------------------------------------

# tidy: Clean up go.mod / go.sum for both modules.
# Run after adding or removing dependencies.
tidy:
	@echo "==> Tidying root module..."
	$(GO) mod tidy
	@echo "==> Tidying scaffolder module..."
	cd tools/scaffolder && $(GO) mod tidy

# clean: Remove all build artifacts.
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf $(BUILD_OUT)/

# ----------------------------------------------------------------------------
# Help
# ----------------------------------------------------------------------------

# help: Print a summary of available targets.
help:
	@echo "Bridgit Sync Engine — Makefile Targets"
	@echo "======================================="
	@echo "  all               Build everything (default)"
	@echo "  build             Build the main engine binary"
	@echo "  build-scaffolder  Build the scaffolder tool"
	@echo "  test              Test the root Go module"
	@echo "  test-scaffolder   Test the scaffolder module"
	@echo "  test-all          Test all Go modules"
	@echo "  run               Run the sync engine"
	@echo "  route             Run the filesystem router once"
	@echo "  tidy              Tidy go.mod for all modules"
	@echo "  clean             Remove build artifacts"
	@echo "  help              Show this message"
