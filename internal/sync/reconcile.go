// Package sync — reconcile.go implements Phase 5: Reconciliation.
//
// Reconciliation detects misalignment between the three source-of-truth
// systems (Registry, GitHub, Local filesystem) and produces a deterministic
// status report. This is strictly read-only — zero mutations to any state.

package sync

import (
	"fmt"
	"os"
	"strings"

	"bridgit/internal/git"
	"bridgit/internal/registry"
)

// ReconcileStatus captures the alignment state of a single repository
// across all three sources. Each entry corresponds to one registry repo
// or one unregistered GitHub repo, with a status code and human-readable details.
type ReconcileStatus struct {
	// RepoID is the registry ID, or the GitHub name if the repo is unregistered.
	RepoID string

	// Status is one of: OK, MISSING_LOCAL, MISSING_GITHUB, UNREGISTERED_GITHUB, DRIFT.
	Status string

	// Details contains human-readable lines explaining the issue.
	Details []string
}

// Reconcile compares the registry against GitHub and local sources to detect drift.
//
// This is a pure function with no side effects — it reads from the three datasets
// and returns a slice of statuses. Each registry repo is checked for local path
// existence, GitHub presence, and cross-link consistency. GitHub repos not found
// in the registry are flagged as unregistered.
func Reconcile(
	reg *registry.Registry,
	githubRepos []GitHubRepo,
	localRepos []LocalRepo,
) []ReconcileStatus {
	// Pre-allocate with a reasonable capacity to avoid repeated slice growth.
	reconcileResults := make([]ReconcileStatus, 0, len(reg.Repos)+len(githubRepos))

	// Build a lookup set of GitHub repo names for O(1) existence checks.
	// This avoids scanning the full slice for every registry entry.
	githubNameSet := buildGitHubNameSet(githubRepos)

	// --- Walk each registry entry and run all applicable checks ---
	for _, registryRepo := range reg.Repos {
		// Track all issues found for this repo. Multiple checks can fail
		// simultaneously (e.g., missing locally AND drifted on GitHub).
		detectedIssues := []ReconcileStatus{}

		// Check 1: Verify that the declared local path actually exists on disk.
		if localIssue := checkLocalPathExists(registryRepo); localIssue != nil {
			detectedIssues = append(detectedIssues, *localIssue)
		}

		// Check 2: Verify that the declared GitHub repo appears in the API response.
		if githubIssue := checkGitHubRepoExists(registryRepo, githubNameSet, len(githubRepos)); githubIssue != nil {
			detectedIssues = append(detectedIssues, *githubIssue)
		}

		// Check 4: Verify that the local git remote agrees with the registry's GitHub name.
		if driftIssue := checkCrossLinkDrift(registryRepo); driftIssue != nil {
			detectedIssues = append(detectedIssues, *driftIssue)
		}

		// If no issues were detected, the repo is fully aligned across all sources.
		if len(detectedIssues) == 0 {
			reconcileResults = append(reconcileResults, ReconcileStatus{
				RepoID: registryRepo.ID,
				Status: "OK",
			})
			continue
		}

		// Append all detected issues for this repo to the master results.
		reconcileResults = append(reconcileResults, detectedIssues...)
	}

	// Check 3: Find GitHub repos that have no corresponding registry entry.
	// This runs separately because it iterates GitHub repos, not registry entries.
	unregisteredResults := checkUnregisteredGitHub(reg, githubRepos)
	reconcileResults = append(reconcileResults, unregisteredResults...)

	return reconcileResults
}

// checkLocalPathExists verifies that a registry repo's declared local path
// actually exists on disk. Returns nil if the path is empty (no local clone
// expected) or if the directory exists.
func checkLocalPathExists(registryRepo registry.Repo) *ReconcileStatus {
	// Skip repos that don't declare a local path — nothing to verify.
	if registryRepo.Local.Path == "" {
		return nil
	}

	// Use os.Stat to check existence without following symlinks deeper than needed.
	_, statError := os.Stat(registryRepo.Local.Path)
	if statError == nil {
		// Directory exists — no issue.
		return nil
	}

	// The declared path is missing. This typically means the repo was moved,
	// deleted, or the registry entry was created from a different machine.
	return &ReconcileStatus{
		RepoID: registryRepo.ID,
		Status: "MISSING_LOCAL",
		Details: []string{
			fmt.Sprintf("expected at %s", registryRepo.Local.Path),
			"directory does not exist",
		},
	}
}

// checkGitHubRepoExists verifies that a registry repo's declared GitHub name
// appears in the fetched GitHub API response. Returns nil if no GitHub name
// is declared or if the repo is found.
func checkGitHubRepoExists(
	registryRepo registry.Repo,
	githubNameSet map[string]bool,
	totalGitHubCount int,
) *ReconcileStatus {
	// Skip repos that don't declare a GitHub identity — nothing to verify.
	if registryRepo.GitHub.Name == "" {
		return nil
	}

	// Check the pre-built set for a case-insensitive match.
	lowerGitHubName := strings.ToLower(registryRepo.GitHub.Name)
	if githubNameSet[lowerGitHubName] {
		return nil
	}

	// The declared GitHub repo was not found in the API response. This could
	// mean the repo was deleted, renamed, or transferred to another owner.
	return &ReconcileStatus{
		RepoID: registryRepo.ID,
		Status: "MISSING_GITHUB",
		Details: []string{
			fmt.Sprintf("expected GitHub repo: %s", registryRepo.GitHub.Name),
			fmt.Sprintf("not found in GitHub API response (%d repos fetched)", totalGitHubCount),
		},
	}
}

// checkUnregisteredGitHub finds GitHub repos that have no corresponding entry
// in the registry. Returns a slice of UNREGISTERED_GITHUB statuses.
//
// Three matching strategies are attempted before declaring a repo unregistered:
// exact GitHub.Name match, case-insensitive ID match, and alias match.
func checkUnregisteredGitHub(
	reg *registry.Registry,
	githubRepos []GitHubRepo,
) []ReconcileStatus {
	unregisteredResults := []ReconcileStatus{}

	for _, githubRepo := range githubRepos {
		// Try to find any registry entry that claims this GitHub repo.
		if registryHasGitHubRepo(reg, githubRepo.Name) {
			continue
		}

		// No registry entry references this GitHub repo by any strategy.
		unregisteredResults = append(unregisteredResults, ReconcileStatus{
			RepoID: githubRepo.Name,
			Status: "UNREGISTERED_GITHUB",
			Details: []string{
				"exists on GitHub but not in registry",
			},
		})
	}

	return unregisteredResults
}

// checkCrossLinkDrift detects when a repo's local git remote disagrees with
// the GitHub name recorded in the registry. This catches cases where someone
// re-pointed a local clone to a different upstream or the registry entry
// was manually edited incorrectly.
func checkCrossLinkDrift(registryRepo registry.Repo) *ReconcileStatus {
	// Drift detection requires both a local path and a GitHub name.
	// Without both, there's nothing to cross-reference.
	if registryRepo.Local.Path == "" || registryRepo.GitHub.Name == "" {
		return nil
	}

	// Verify the local path exists before shelling out to git.
	// A missing directory would cause a confusing git error.
	if _, statError := os.Stat(registryRepo.Local.Path); statError != nil {
		// Already caught by checkLocalPathExists — skip to avoid duplicate noise.
		return nil
	}

	// Shell out to git to read the actual origin remote URL.
	remoteInfo, remoteError := git.DetectGitHubRemote(registryRepo.Local.Path)
	if remoteError != nil {
		// If git can't determine the remote, we can't check for drift.
		// This isn't necessarily an error — some repos may lack remotes.
		return nil
	}

	// Compare the git remote's repo name against the registry's declared name.
	// Case-insensitive because GitHub URLs are case-insensitive.
	if strings.EqualFold(remoteInfo.Repo, registryRepo.GitHub.Name) {
		// Names match — no drift detected.
		return nil
	}

	// The local repo points to a different GitHub repo than the registry expects.
	return &ReconcileStatus{
		RepoID: registryRepo.ID,
		Status: "DRIFT",
		Details: []string{
			fmt.Sprintf("registry says GitHub name: %s", registryRepo.GitHub.Name),
			fmt.Sprintf("local git remote says: %s", remoteInfo.Repo),
		},
	}
}

// buildGitHubNameSet creates a case-insensitive lookup set from a slice of
// GitHub repos. This trades a small amount of memory for O(1) lookups
// instead of O(n) scans during reconciliation.
func buildGitHubNameSet(githubRepos []GitHubRepo) map[string]bool {
	nameSet := make(map[string]bool, len(githubRepos))

	for _, githubRepo := range githubRepos {
		nameSet[strings.ToLower(githubRepo.Name)] = true
	}

	return nameSet
}

// registryHasGitHubRepo checks whether any registry entry claims the given
// GitHub repo name using three progressively looser matching strategies.
//
// This prevents false UNREGISTERED_GITHUB results for repos that exist in
// the registry under a different ID or alias.
func registryHasGitHubRepo(reg *registry.Registry, githubName string) bool {
	lowerGitHubName := strings.ToLower(githubName)

	for _, registryRepo := range reg.Repos {
		// Strategy 1: Exact match on the GitHub.Name field.
		if strings.EqualFold(registryRepo.GitHub.Name, githubName) {
			return true
		}

		// Strategy 2: Case-insensitive match on the registry ID itself.
		// Handles cases where the ID mirrors the GitHub name.
		if strings.ToLower(registryRepo.ID) == lowerGitHubName {
			return true
		}

		// Strategy 3: Check all registered aliases for a name match.
		for _, aliasName := range registryRepo.Aliases.Names {
			if strings.ToLower(aliasName) == lowerGitHubName {
				return true
			}
		}
	}

	// No registry entry references this GitHub repo by any strategy.
	return false
}
