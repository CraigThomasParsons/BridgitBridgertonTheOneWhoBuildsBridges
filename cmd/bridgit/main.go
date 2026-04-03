// Package main is the entry point for the Bridgit sync engine binary.
//
// Bridgit orchestrates file-based pipeline synchronization across multiple
// systems (ChatGPT projects, GitHub repos, local filesystems) using a
// deterministic inbox/outbox pattern.
package main

import (
	"log"

	"bridgit/internal/app"
)

// main delegates to the application run function and enforces
// fatal termination on any error to prevent silent failures.
//
// Fatal errors exit with non-zero status codes, which is critical
// for systemd service monitoring and CI/CD pipeline detection.
func main() {
	// Delegate to the application layer for orchestration.
	// This separation keeps main.go minimal and testable.
	if err := app.Run(); err != nil {
		// Fatal immediately on error to trigger systemd restart policies
		// and ensure operators are alerted via log aggregation.
		log.Fatal(err)
	}
}
