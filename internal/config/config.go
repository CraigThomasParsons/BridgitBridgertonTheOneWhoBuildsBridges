// Package config provides configuration loading and management for Bridgit.
//
// Currently uses hardcoded values for development. Production deployments
// should replace this with environment variable parsing or a proper config
// file loader (e.g., Viper or envconfig).
package config

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

	// EnableProvisioning controls whether the provisioning phase creates
	// local directories, GitHub repos, and links them together. Default false
	// — opt-in to prevent unexpected filesystem and GitHub mutations.
	EnableProvisioning bool

	// LLMAPIKey authenticates requests to the LLM API (OpenAI-compatible).
	// Read from LLM_API_KEY environment variable, falls back to GROQ_API_KEY.
	// If empty, LLM-based fuzzy matching is silently skipped.
	LLMAPIKey string

	// LLMBaseURL is the base URL for the OpenAI-compatible chat completions API.
	// Defaults to Groq's endpoint. Works with any OpenAI-compatible provider.
	LLMBaseURL string

	// LLMModel specifies which model to use for fuzzy matching.
	// Defaults to Groq's llama-3.3-70b-versatile — fast and capable.
	LLMModel string
}

// getEnvOrDefault returns the value of an environment variable, or a fallback
// if the variable is empty or unset. Used for optional config with sane defaults.
func getEnvOrDefault(envKey string, fallback string) string {
	envValue := os.Getenv(envKey)
	if envValue == "" {
		return fallback
	}
	return envValue
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
		EnableIntake:       false,
		EnableProvisioning: false,
		LLMAPIKey:  getEnvOrDefault("LLM_API_KEY", os.Getenv("GROQ_API_KEY")),
		LLMBaseURL: getEnvOrDefault("LLM_BASE_URL", "https://api.groq.com/openai/v1"),
		LLMModel:   getEnvOrDefault("LLM_MODEL", "llama-3.3-70b-versatile"),
	}
}
