// Package main is the entry point for the scaffolder CLI tool.
//
// Scaffolder reads tree-format directory structure files (e.g., from `tree` command
// or hand-written) and materializes them as real directories and empty files on disk.
// This accelerates project setup by converting visual directory layouts into actual
// filesystem structures.
package main

import (
	"log"
	"os"

	"scaffolder/internal/runner"
)

// main validates command-line arguments and delegates to the runner.
//
// Expects exactly one argument: the path to a tree-format file describing
// the desired directory structure. Fatal errors trigger immediate exit with
// non-zero status codes for shell script integration.
func main() {
	// Validate that a tree file path was provided.
	// Without this, we cannot proceed since there's no structure to scaffold.
	if len(os.Args) < 2 {
		// Exit immediately with usage message.
		// Non-zero exit code signals shell scripts that invocation failed.
		log.Fatal("Usage: scaffolder <tree-file>")
	}

	// Delegate to the runner with the tree file path.
	// The runner orchestrates parsing and filesystem creation.
	if err := runner.Run(os.Args[1]); err != nil {
		// Fatal on any error to prevent partial scaffolds.
		// Operators need to see clear error messages for debugging.
		log.Fatal(err)
	}
}
