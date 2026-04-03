package sync

// GitHubRepo represents a single repository from GitHub.
//
// This struct captures the essential metadata for linking GitHub repos
// to Bridgit's registry. Additional fields (description, stars, language)
// can be added as the use case expands.
type GitHubRepo struct {
	// Name is the repository name (e.g., "BridgitBridgerton").
	Name string

	// URL is the full clone URL for git operations.
	// Example: "https://github.com/CraigThomasParsons/BridgitBridgerton.git"
	URL string
}

// FetchGitHubRepos retrieves all repositories for the given GitHub owner.
//
// This is currently a stub implementation that returns an empty list.
// The real implementation will use the GitHub API (likely via go-github)
// to list repos for the specified owner (user or organization).
//
// TODO: Integrate github.com/google/go-github for API calls.
// TODO: Add authentication token handling (env var or config).
// TODO: Handle pagination for users with >100 repos.
func FetchGitHubRepos(owner string) ([]GitHubRepo, error) {
	// Stub: return empty list until GitHub API integration is complete.
	// This allows the engine to compile and run without blocking.
	return []GitHubRepo{}, nil
}
