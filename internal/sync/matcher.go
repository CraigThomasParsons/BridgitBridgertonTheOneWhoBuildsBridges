// Package sync — matcher.go implements Phase 3 identity resolution.
//
// The matcher links local directories, GitHub repositories, and registry
// entries into unified identities. It uses a deterministic priority pipeline:
//
//  1. Git remote match (highest confidence — the repo itself declares its identity)
//  2. Registry alias match (medium confidence — human-curated names)
//  3. Normalized name match (low confidence — string similarity heuristic)
//  4. Unresolved (no match found)
//
// Bridgit never mutates the filesystem. The matcher only reads git remotes
// and compares strings — it answers "what is this?" without changing anything.
package sync

import (
	"strings"

	"bridgit/internal/git"
	"bridgit/internal/registry"
)

// MatchResult captures the outcome of attempting to resolve a local repo's
// identity against the registry and GitHub. Each local directory produces
// exactly one MatchResult.
type MatchResult struct {
	// LocalPath is the absolute path to the directory that was matched.
	LocalPath string

	// LocalName is the directory basename before normalization.
	LocalName string

	// RepoID is the registry ID that this local path resolved to.
	// Empty when Strategy is "none".
	RepoID string

	// GitHubName is the repo name extracted from the git remote, if any.
	// Empty when the directory has no origin remote or isn't on GitHub.
	GitHubName string

	// Strategy records which matching rule produced the result.
	// One of: "git", "alias", "normalized", "none".
	Strategy string

	// Confidence indicates how trustworthy the match is.
	// One of: "high", "medium", "low", "none".
	Confidence string
}

// MatchLocal runs the full identity resolution pipeline for a single local repo.
//
// The pipeline is ordered by confidence — the first match wins:
//  1. Git remote: shell out to `git remote get-url origin`, parse the GitHub
//     owner/repo, and look for a registry entry with matching GitHub.Name.
//  2. Alias: compare the directory name against all repo.Aliases.Names entries.
//  3. Normalized: kebab-case the directory name and compare against repo IDs.
//  4. None: no match — this repo is truly unresolved.
//
// githubRepos is the pre-fetched list from the GitHub API, used for cross-
// referencing when a git remote points at a repo not yet in the registry.
func MatchLocal(local LocalRepo, reg *registry.Registry, githubRepos []GitHubRepo) MatchResult {
	result := MatchResult{
		LocalPath:  local.Path,
		LocalName:  local.Name,
		Strategy:   "none",
		Confidence: "none",
	}

	// --- Strategy 1: Git remote match (highest confidence) ---
	// The repo's own .git/config declares where it came from.
	// This is the most reliable signal because it's set by git clone.
	remoteInfo, remoteError := git.DetectGitHubRemote(local.Path)
	if remoteError == nil && remoteInfo.Repo != "" {
		result.GitHubName = remoteInfo.Repo

		// Search registry for a repo with matching GitHub metadata.
		matchedRepo := findRepoByGitHubName(reg, remoteInfo.Repo)
		if matchedRepo != nil {
			result.RepoID = matchedRepo.ID
			result.Strategy = "git"
			result.Confidence = "high"
			return result
		}

		// No registry entry yet, but we know the GitHub name.
		// Check if this GitHub repo exists in the fetched API list.
		for _, ghRepo := range githubRepos {
			if strings.EqualFold(ghRepo.Name, remoteInfo.Repo) {
				// The repo exists on GitHub but isn't in the registry yet.
				// Use the GitHub name as the resolved ID.
				result.RepoID = remoteInfo.Repo
				result.Strategy = "git"
				result.Confidence = "high"
				return result
			}
		}

		// Git remote points at a GitHub repo we don't know about.
		// Still high confidence — the remote is authoritative.
		result.RepoID = remoteInfo.Repo
		result.Strategy = "git"
		result.Confidence = "high"
		return result
	}

	// --- Strategy 2: Alias match (medium confidence) ---
	// Check the directory name against human-curated alias lists.
	// Aliases are set during adoption or manual registry editing.
	lowerLocalName := strings.ToLower(local.Name)
	for _, repo := range reg.Repos {
		for _, aliasName := range repo.Aliases.Names {
			// Compare case-insensitively for resilience against naming variance.
			if strings.ToLower(aliasName) == lowerLocalName {
				result.RepoID = repo.ID
				result.Strategy = "alias"
				result.Confidence = "medium"
				return result
			}
		}
	}

	// --- Strategy 3: Normalized name match (low confidence) ---
	// Kebab-case the directory name and compare against registry IDs.
	// This catches cases where the folder is "Personal_Executive_Function"
	// and the registry ID is "personal-executive-function".
	normalizedLocalName := normalizeID(local.Name)
	for _, repo := range reg.Repos {
		if repo.ID == normalizedLocalName {
			result.RepoID = repo.ID
			result.Strategy = "normalized"
			result.Confidence = "low"
			return result
		}
	}

	// --- Strategy 4: No match ---
	// This local directory doesn't correspond to any known identity.
	return result
}

// MatchAll runs identity resolution across every local repo and returns
// the complete set of results. This is the main entry point called by
// the engine during Phase 3.
//
// Results are returned in the same order as the input localRepos slice.
// Ignored directories (dotfiles) are excluded from the results entirely.
func MatchAll(localRepos []LocalRepo, reg *registry.Registry, githubRepos []GitHubRepo) []MatchResult {
	var results []MatchResult

	for _, local := range localRepos {
		// Skip dotfiles and system directories — not real projects.
		if isIgnored(local.Name) {
			continue
		}

		// Run the full matching pipeline for this directory.
		result := MatchLocal(local, reg, githubRepos)
		results = append(results, result)
	}

	return results
}

// findRepoByGitHubName searches the registry for an entry whose GitHub.Name
// matches the given name (case-insensitive). Returns a pointer to the matching
// Repo or nil if no match exists.
//
// This is separate from findRepoByNameMatch because it specifically targets
// the GitHub.Name field — the most reliable identifier from git remotes.
func findRepoByGitHubName(reg *registry.Registry, githubName string) *registry.Repo {
	lowerName := strings.ToLower(githubName)

	for i := range reg.Repos {
		// Match against the GitHub name field specifically.
		// This field is set during enrichment when a GitHub API match is found.
		if strings.ToLower(reg.Repos[i].GitHub.Name) == lowerName {
			return &reg.Repos[i]
		}
	}

	return nil
}
