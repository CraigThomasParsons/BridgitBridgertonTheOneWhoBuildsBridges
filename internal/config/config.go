// Package config provides configuration loading and management for Bridgit.
//
// Currently uses hardcoded values for development. Production deployments
// should replace this with environment variable parsing or a proper config
// file loader (e.g., Viper or envconfig).
package config

// Importing os for future environment variable support, currently unused.
import "os"

// Config holds all runtime configuration required for Bridgit execution.
//
// CodeRoot defines the local filesystem path where repositories are scanned.
// RegistryPath points to the TOML file tracking known repositories.
// GitHubOwner specifies which GitHub account to fetch repositories from.
type Config struct {
	// CodeRoot is the absolute path to the directory containing local repos.
	// All subdirectories here are scanned and compared against the registry.
	CodeRoot string

	// RegistryPath is the relative or absolute path to the TOML registry file.
	// This file persists the authoritative mapping of repos across sources.
	RegistryPath string

	// GitHubOwner is the GitHub username or organization to fetch repos from.
	// Used by the GitHub sync adapter to list remote repositories.
	GitHubOwner string

	// AutoAdopt controls whether unregistered local repos are automatically
	// added to the registry. When false (default), Bridgit only reports
	// candidates without mutating state — a safe preview mode.
	AutoAdopt bool

	// ArchivePath is the directory where bridge job outputs are stored.
	// Projection reads from here and copies artifacts into repo docs/ folders.
	// Defaults to runtime/archive/ relative to the registry file.
	ArchivePath string

	// EnableProjection controls whether Phase 6 artifact projection runs.
	// Default false — opt-in like AutoAdopt to prevent unexpected writes.
	EnableProjection bool

	// InboxPath is the directory where the router deposits packages for processing.
	// The intake phase reads from here and generates manifests for projection.
	// Defaults to runtime/inbox/ relative to the project root.
	InboxPath string

	// FailedPath is the directory where packages that fail intake processing are moved.
	// Operators inspect this directory to diagnose pipeline failures.
	// Defaults to runtime/failed/ relative to the project root.
	FailedPath string

	// EnableIntake controls whether the intake phase processes inbox packages.
	// Default false — opt-in to prevent unexpected filesystem mutations.
	EnableIntake bool
}

// Load returns a Config instance with hardcoded development values.
//
// This is a temporary implementation for prototyping. Production code should:
// - Read from environment variables (e.g., BRIDGIT_CODE_ROOT)
// - Support CLI flag overrides (e.g., --code-root /custom/path)
// - Validate paths exist and are readable before returning
func Load() Config {

	// Grab the user's home directory to construct a plausible default code root.
	home, err := os.UserHomeDir()

	if err != nil {
		// fallback (very unlikely to fail, but safe)
		home = "/tmp"
	}

	// Return hardcoded values for initial development.
	// These paths are specific to the development environment.
	// AutoAdopt defaults to false so the first run is always a safe preview.
	// Flip to true once you've reviewed the candidate output.
	return Config{
		CodeRoot:         home + "/Code",
		RegistryPath:     "./registry/repo_registry.toml",
		GitHubOwner:      "CraigThomasParsons",
		AutoAdopt:        false,
		ArchivePath:      "./runtime/archive",
		EnableProjection: false,
		InboxPath:        "./runtime/inbox",
		FailedPath:       "./runtime/failed",
		EnableIntake:     false,
	}
}
