// Package report provides a simple line-based report builder for Bridgit.
//
// Reports accumulate human-readable status lines during sync execution
// and render to stdout for operator visibility. This is preferred over
// structured logging for quick operational feedback.
package report

import "strings"

// Report accumulates status lines during Bridgit sync execution.
//
// This is a simple append-only data structure optimized for rendering
// to stdout after a sync run completes. Lines are stored in order.
type Report struct {
	// lines holds all accumulated report entries.
	// Unexported to enforce Add() as the only mutation interface.
	lines []string
}

// New creates a fresh Report instance with no lines.
//
// Reports start empty and accumulate lines via Add() calls
// throughout the sync engine's execution.
func New() *Report {
	return &Report{}
}

// Add appends a single line to the report.
//
// Lines are stored in the order they are added, preserving
// the logical flow of sync operations for the final render.
// Newlines within individual lines are not supported.
func (r *Report) Add(line string) {
	// Append directly to the slice.
	// No deduplication or validation is performed.
	r.lines = append(r.lines, line)
}

// Render joins all accumulated lines into a single newline-delimited string.
//
// This produces the final output suitable for fmt.Println or log writing.
// The result is deterministic and suitable for diffing across runs.
func (r *Report) Render() string {
	// Join with Unix newlines for cross-platform consistency.
	return strings.Join(r.lines, "\n")
}
