// Package git provides local git repository inspection and provisioning for Bridgit.
//
// Read-only operations (remote detection, URL parsing) live in git.go.
// Write operations (clone, init, push) for the provisioning phase live in write.go.
package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// RemoteInfo holds the parsed components of a git remote URL.
//
// Only populated for GitHub-style remotes (HTTPS or SSH).
// Non-GitHub remotes are still captured in RawURL but Owner/Repo
// will be empty.
type RemoteInfo struct {
	// RawURL is the unmodified output of `git remote get-url origin`.
	RawURL string

	// Owner is the GitHub user or org (e.g., "CraigThomasParsons").
	Owner string

	// Repo is the repository name without .git suffix (e.g., "Krax").
	Repo string
}

// GetRemoteURL runs `git remote get-url origin` inside the given directory
// and returns the raw URL string. Returns an error if the directory is not
// a git repo or has no origin remote configured.
//
// This shells out to git rather than parsing .git/config directly because
// git handles all edge cases (worktrees, submodules, overrides).
func GetRemoteURL(repoPath string) (string, error) {
	// Build the git command targeting the specific directory.
	// Using -C avoids needing to chdir, which is safer for concurrent use.
	gitCommand := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")

	// Capture stdout; stderr goes to /dev/null since we handle errors ourselves.
	rawOutput, err := gitCommand.Output()
	if err != nil {
		return "", fmt.Errorf("no origin remote in %s: %w", repoPath, err)
	}

	// Trim whitespace and newlines from the git output.
	remoteURL := strings.TrimSpace(string(rawOutput))
	if remoteURL == "" {
		return "", fmt.Errorf("empty origin URL in %s", repoPath)
	}

	return remoteURL, nil
}

// ParseRemoteURL extracts owner and repo name from a GitHub remote URL.
//
// Supports both HTTPS and SSH formats:
//
//	https://github.com/Owner/Repo.git
//	git@github.com:Owner/Repo.git
//
// Returns a RemoteInfo with RawURL always set. Owner and Repo are only
// populated if the URL matches a recognized GitHub pattern.
func ParseRemoteURL(rawURL string) RemoteInfo {
	info := RemoteInfo{RawURL: rawURL}

	// Try HTTPS format first: https://github.com/Owner/Repo.git
	if strings.Contains(rawURL, "github.com/") {
		// Split on "github.com/" to isolate the owner/repo path segment.
		parts := strings.SplitN(rawURL, "github.com/", 2)
		if len(parts) == 2 {
			info.Owner, info.Repo = parseOwnerRepo(parts[1])
		}
		return info
	}

	// Try SSH format: git@github.com:Owner/Repo.git
	if strings.Contains(rawURL, "github.com:") {
		// Split on "github.com:" to isolate the owner/repo path segment.
		parts := strings.SplitN(rawURL, "github.com:", 2)
		if len(parts) == 2 {
			info.Owner, info.Repo = parseOwnerRepo(parts[1])
		}
		return info
	}

	// Not a GitHub URL — return with RawURL only.
	return info
}

// parseOwnerRepo splits an "Owner/Repo.git" path into its components.
//
// Strips the .git suffix and any trailing slashes. Returns empty strings
// if the path doesn't contain exactly one slash separating owner and repo.
func parseOwnerRepo(path string) (string, string) {
	// Strip .git suffix if present.
	cleanPath := strings.TrimSuffix(path, ".git")

	// Remove any trailing slash from copy-paste artifacts.
	cleanPath = strings.TrimRight(cleanPath, "/")

	// Split into owner and repo on the first slash.
	ownerAndRepo := strings.SplitN(cleanPath, "/", 2)
	if len(ownerAndRepo) != 2 || ownerAndRepo[0] == "" || ownerAndRepo[1] == "" {
		return "", ""
	}

	// Strip any nested path components — only the first segment is the repo name.
	// e.g., "Owner/Repo/tree/main" → Owner, Repo
	repoName := ownerAndRepo[1]
	if slashIndex := strings.Index(repoName, "/"); slashIndex != -1 {
		repoName = repoName[:slashIndex]
	}

	return ownerAndRepo[0], repoName
}

// DetectGitHubRemote is a convenience function that combines GetRemoteURL
// and ParseRemoteURL into a single call. Returns a fully populated RemoteInfo
// or an error if the directory has no origin remote.
//
// This is the primary entry point for the matcher — one call per local repo.
func DetectGitHubRemote(repoPath string) (RemoteInfo, error) {
	// Resolve symlinks so the reported path is canonical.
	resolvedPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		resolvedPath = repoPath
	}

	// Fetch the raw remote URL from git.
	rawURL, err := GetRemoteURL(resolvedPath)
	if err != nil {
		return RemoteInfo{}, err
	}

	// Parse the URL into structured owner/repo fields.
	return ParseRemoteURL(rawURL), nil
}
