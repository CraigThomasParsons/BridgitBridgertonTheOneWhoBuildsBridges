// Package builder materializes parsed tree nodes into actual filesystem structures.
//
// This package creates directories and empty files on disk based on the Node
// structures provided by the parser. It handles parent directory creation,
// permission setting, and idempotent file creation (won't overwrite existing files).
package builder

import (
	"os"
	"path/filepath"
	"strings"

	"scaffolder/internal/parser"
)

// Build creates all directories and files represented by the given nodes.
//
// This function iterates through nodes in order and materializes each as either:
// - Directory: Created with 0755 permissions (rwxr-xr-x) for standard access
// - File: Created as empty with default permissions, but only if it doesn't exist
//
// Parent directories are created automatically as needed. Existing files are
// never overwritten, allowing safe re-runs without destroying work.
func Build(nodes []parser.Node) error {
	// Process each node sequentially.
	// Order matters since parent directories must exist before children.
	for _, n := range nodes {
		// Check if this node represents a directory (trailing slash).
		if n.IsDir {
			// Remove the trailing slash for MkdirAll.
			// MkdirAll expects a clean path without trailing separators.
			err := os.MkdirAll(strings.TrimSuffix(n.Path, "/"), 0755)
			if err != nil {
				// Directory creation failure is fatal.
				// Could be permissions, disk space, or invalid path.
				return err
			}
		} else {
			// This is a file node - ensure parent directory exists first.
			// Extract the parent directory path from the file path.
			dir := filepath.Dir(n.Path)

			// Create parent directory if it's not the current directory.
			if dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					// Parent directory creation is critical for file creation.
					return err
				}
			}

			// Check if the file already exists.
			// We only create if missing to avoid overwriting existing work.
			if _, err := os.Stat(n.Path); os.IsNotExist(err) {
				// Create an empty file at this path.
				// Default permissions (0644) are applied automatically.
				f, err := os.Create(n.Path)
				if err != nil {
					// File creation failure is fatal.
					// Could be permissions or disk space issues.
					return err
				}
				// Close immediately since we only want empty files.
				// No content is written at this stage.
				f.Close()
			}
			// If file exists, skip silently to preserve existing content.
		}
	}

	// All nodes successfully materialized.
	return nil
}
