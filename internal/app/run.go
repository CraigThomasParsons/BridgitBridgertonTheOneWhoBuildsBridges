// Package app provides the top-level application orchestration for Bridgit.
//
// This layer coordinates configuration loading, registry management, sync engine
// execution, and report rendering. It serves as the bridge between the main
// entry point and the domain-specific sync logic.
package app

import (
	"fmt"

	"bridgit/internal/config"
	"bridgit/internal/registry"
	"bridgit/internal/sync"
)

// Run executes the complete Bridgit synchronization workflow from start to finish.
//
// This function loads configuration, hydrates the repository registry from disk,
// runs the multi-source sync engine, renders a human-readable report to stdout,
// and persists any registry updates back to disk. Errors at any stage bubble up
// to trigger fatal termination in main.
func Run() error {
	// Load hardcoded configuration values.
	// TODO: Replace with environment variables or CLI flags for production.
	cfg := config.Load()

	// Attempt to load the existing registry from TOML on disk.
	// First runs gracefully create an empty registry instead of failing.
	reg, err := registry.Load(cfg.RegistryPath)
	if err != nil {
		// Bubble error immediately to halt execution.
		// Registry corruption or permission issues must not proceed.
		return err
	}

	// Initialize the sync engine with configuration and registry.
	// The engine coordinates fetching from multiple sources (Chat, GitHub, local).
	engine := sync.NewEngine(cfg, reg)

	// Execute the synchronization scan across all configured sources.
	// This populates the report with discovered repos and orphaned paths.
	report, err := engine.Run()
	if err != nil {
		// Fail fast on sync errors to prevent incomplete state persistence.
		return err
	}

	// Render the report to stdout for operator visibility.
	// This provides immediate feedback on orphaned folders and sync counts.
	fmt.Println(report.Render())

	// Persist the updated registry back to disk.
	// This ensures discovered repos are tracked for future runs.
	return registry.Save(cfg.RegistryPath, reg)
}
