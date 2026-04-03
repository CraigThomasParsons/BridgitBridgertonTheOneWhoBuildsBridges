// Package runner orchestrates the scaffolding workflow from parsing to building.
//
// This layer coordinates the parser and builder to create a complete pipeline:
// tree file → parsed nodes → filesystem materialization → success report.
// It serves as the bridge between main and the domain-specific components.
package runner

import (
	"fmt"

	"scaffolder/internal/builder"
	"scaffolder/internal/parser"
)

// Run executes the complete scaffolding pipeline for the given tree file.
//
// This function orchestrates two phases:
// 1. Parse the tree file into structured Node objects
// 2. Build the actual directories and files from those nodes
//
// Errors at any stage are fatal and propagated immediately to prevent
// partial scaffolds that could confuse project initialization.
func Run(treeFile string) error {
	// Parse the tree file into Node structs.
	// Each node represents either a directory or file path.
	nodes, err := parser.Parse(treeFile)
	if err != nil {
		// Parsing failure means the tree file is corrupt or missing.
		// Bubble the error to main for fatal termination.
		return err
	}

	// Materialize the parsed nodes as actual filesystem entities.
	// This creates directories and empty files in the correct structure.
	if err := builder.Build(nodes); err != nil {
		// Build failure means filesystem permissions or I/O issues.
		// Stop immediately to prevent partial scaffolds.
		return err
	}

	// Report success to stdout for operator confirmation.
	// This provides immediate feedback that the scaffold completed.
	fmt.Println("Scaffold complete")
	return nil
}
