// Package registry defines the data models for Bridgit's repository tracking system.
//
// The registry serves as the authoritative source of truth for mapping repositories
// across multiple systems (ChatGPT projects, GitHub repos, local filesystems).
// It persists to TOML format for human readability and version control friendliness.
package registry

// Repo represents a single repository tracked across multiple sources.
//
// Each repo can have ChatGPT project metadata, GitHub metadata, local filesystem
// metadata, and aliases. Not all fields are required - a repo might exist in only
// one source initially and get enriched as Bridgit discovers it elsewhere.
type Repo struct {
	// ID is the unique identifier for this repository across all sources.
	// Typically matches the primary source's ID (e.g., GitHub repo name).
	ID string `toml:"id"`

	// Chat holds ChatGPT project-specific metadata.
	// Only populated when this repo is linked to a ChatGPT project.
	Chat struct {
		// ProjectName is the human-readable name in ChatGPT Projects.
		ProjectName string `toml:"project_name"`

		// ProjectID is the unique ChatGPT project identifier.
		// Used for API calls to fetch conversations and artifacts.
		ProjectID string `toml:"project_id"`
	} `toml:"chat"`

	// GitHub holds GitHub-specific metadata for this repository.
	// Only populated when this repo exists on GitHub.
	GitHub struct {
		// Name is the GitHub repository name (e.g., "BridgitBridgerton").
		Name string `toml:"name"`

		// URL is the full clone URL (e.g., "https://github.com/user/repo.git").
		// Used for git clone and remote verification operations.
		URL string `toml:"url"`
	} `toml:"github"`

	// Local holds local filesystem metadata for this repository.
	// Only populated when this repo exists on the local machine.
	Local struct {
		// Path is the absolute path to the repository on disk.
		// Used for git operations and file-based sync.
		Path string `toml:"path"`
	} `toml:"local"`

	// Aliases holds alternative names and paths for fuzzy matching.
	// Allows flexible discovery when repo names differ across systems.
	Aliases struct {
		// Names contains alternative repository names for matching.
		// Example: ["bridgit", "bridgit-sync-engine", "BridgitBridgerton"].
		Names []string `toml:"names"`

		// Paths contains alternative filesystem paths for matching.
		// Useful when repos are symlinked or moved.
		Paths []string `toml:"paths"`
	} `toml:"aliases"`
}

// Registry is the top-level container for all tracked repositories.
//
// Persisted to TOML format at the path specified in Config.RegistryPath.
// Loaded at startup and saved after each sync run to persist discoveries.
type Registry struct {
	// Repos is the list of all repositories known to Bridgit.
	// Indexed by Repo.ID for fast lookups during sync operations.
	Repos []Repo `toml:"repo"`
}
