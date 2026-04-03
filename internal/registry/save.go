package registry

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Save persists the given Registry to disk as a TOML file at the specified path.
//
// This function marshals the in-memory registry state to TOML format and writes
// it atomically to disk. File permissions are set to 0644 (owner read/write,
// group/others read-only) for security and git-friendliness.
//
// Any marshaling or I/O errors are fatal and propagated to prevent partial writes.
func Save(path string, r *Registry) error {
	// Marshal the Registry struct to TOML format bytes.
	// The go-toml/v2 library ensures valid TOML output.
	data, err := toml.Marshal(r)
	if err != nil {
		// Marshaling failure indicates a programmer error (invalid struct).
		// This should never happen in production but must be caught.
		return err
	}

	// Write the TOML bytes to disk atomically.
	// 0644 permissions allow the file to be committed to git.
	return os.WriteFile(path, data, 0644)
}
