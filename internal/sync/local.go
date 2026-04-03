package sync

import "os"

// LocalRepo represents a single directory on the local filesystem.
//
// This struct captures directories that *may* be git repositories,
// but no git validation is performed at this layer. The engine treats
// all subdirectories under CodeRoot as potential repos for registry reconciliation.
type LocalRepo struct {
	// Name is the directory name (not the full path).
	Name string

	// Path is the absolute path to the directory.
	Path string
}

// ScanLocal scans the given root directory for immediate subdirectories.
//
// This function performs a shallow scan (one level deep) to discover
// potential repositories. It does NOT recurse into nested directories
// or validate whether directories are git repos - that filtering happens
// elsewhere in the pipeline.
//
// Directory read errors are fatal since CodeRoot must be accessible.
func ScanLocal(root string) ([]LocalRepo, error) {
	// Read all entries in the root directory.
	// This is a shallow scan - we do not recurse into subdirectories.
	entries, err := os.ReadDir(root)
	if err != nil {
		// Fail immediately if CodeRoot is inaccessible.
		// Operators must have read permissions on this directory.
		return nil, err
	}

	// Pre-allocate a slice to hold discovered repos.
	// Exact capacity is unknown but likely small (<100 repos).
	var repos []LocalRepo

	// Iterate through all directory entries.
	for _, e := range entries {
		// Only consider directories, not files.
		// Symlinks are resolved by os.ReadDir automatically.
		if e.IsDir() {
			// Construct the LocalRepo struct with name and full path.
			// Path construction uses simple concatenation (TODO: use filepath.Join).
			repos = append(repos, LocalRepo{
				Name: e.Name(),
				Path: root + "/" + e.Name(),
			})
		}
	}

	// Return all discovered directories as potential repos.
	return repos, nil
}
