package registry

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Load reads the repository registry from a TOML file at the given path.
//
// If the file does not exist (typical on first run), this function returns
// an empty Registry instead of failing. This graceful degradation allows
// Bridgit to bootstrap itself on fresh installations.
//
// TOML parsing errors are fatal and propagated to the caller, since corrupted
// registry files indicate a serious problem requiring manual intervention.
func Load(path string) (*Registry, error) {
	// Attempt to read the entire registry file into memory.
	// Small registry files (typically <1MB) make this safe.
	data, err := os.ReadFile(path)
	if err != nil {
		// First run case: registry file does not exist yet.
		// Return an empty registry to allow bootstrapping.
		// Note: This swallows the error intentionally because missing files
		// are expected behavior on first execution.
		return &Registry{}, nil
	}

	// Parse the TOML data into a Registry struct.
	// The go-toml/v2 library handles all TOML spec compliance.
	var r Registry
	if err := toml.Unmarshal(data, &r); err != nil {
		// TOML corruption is fatal - do not proceed with invalid state.
		// Operators must fix or delete the corrupted registry manually.
		return nil, err
	}

	// Successfully loaded and parsed the registry from disk.
	return &r, nil
}
