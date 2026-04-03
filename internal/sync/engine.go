// Package sync implements the core synchronization engine for Bridgit.
//
// The engine coordinates fetching from multiple sources (ChatGPT projects,
// GitHub repos, local filesystems) and reconciles them against the registry
// to detect orphaned repos, auto-register new discoveries, and cross-link
// repos that exist in multiple sources.
package sync

import (
	"fmt"
	"path/filepath"
	"strings"

	"bridgit/internal/config"
	"bridgit/internal/registry"
	"bridgit/internal/report"
)

// Engine orchestrates multi-source synchronization and reconciliation.
//
// It holds references to configuration and the registry, allowing it to
// fetch from external sources, compare against known state, and mutate
// the registry with new discoveries.
type Engine struct {
	// cfg contains runtime configuration like code root and GitHub owner.
	cfg *config.Config

	// reg is the shared registry of known repositories.
	// The engine reads from and writes to this during reconciliation.
	reg *registry.Registry
}

// NewEngine creates a new sync Engine with the given configuration and registry.
//
// The config and registry are captured by pointer to allow the engine to
// inspect and mutate state without copying large structures.
func NewEngine(cfg config.Config, reg *registry.Registry) *Engine {
	// Store pointers to avoid copying and enable shared state access.
	return &Engine{
		cfg: &cfg,
		reg: reg,
	}
}

// Run executes the full synchronization workflow across all sources.
//
// This function performs four phases:
//  1. Fetch: Pull data from ChatGPT projects, GitHub API, and local filesystem
//  2. Register: Auto-register newly discovered local repos into the registry
//  3. Enrich: Cross-link registry entries with GitHub and ChatProjects data
//  4. Report: Accumulate findings for operator review
//
// Errors from individual sources are logged as warnings in the report,
// allowing other sources to proceed even if one fails.
func (e *Engine) Run() (*report.Report, error) {
	// Initialize a fresh report to accumulate findings.
	r := report.New()

	// Add the report header for visual clarity in stdout.
	r.Add("== Bridgit Sync Report ==")
	r.Add("")

	// --- Phase 1: Fetch from all three sources ---
	// Each source is fetched independently; failures are reported but non-fatal.

	// Fetch ChatGPT projects from the bridge outbox.
	chatProjects, chatError := FetchChatProjects()
	if chatError != nil {
		r.Add(fmt.Sprintf("WARNING: Chat Projects fetch failed: %v", chatError))
	}

	// Fetch GitHub repos via the REST API.
	githubRepos, githubError := FetchGitHubRepos(e.cfg.GitHubOwner)
	if githubError != nil {
		r.Add(fmt.Sprintf("WARNING: GitHub fetch failed: %v", githubError))
	}

	// Scan local directories under CodeRoot.
	localRepos, localError := ScanLocal(e.cfg.CodeRoot)
	if localError != nil {
		// Local scan failure is more serious since it's purely filesystem.
		return nil, fmt.Errorf("failed to scan local repos at %s: %w", e.cfg.CodeRoot, localError)
	}

	// Report the count of repositories discovered in each source.
	// Provides quick feedback on whether sources are responding.
	r.Add(fmt.Sprintf("Chat Projects: %d", len(chatProjects)))
	r.Add(fmt.Sprintf("GitHub Repos:  %d", len(githubRepos)))
	r.Add(fmt.Sprintf("Local Repos:   %d", len(localRepos)))
	r.Add("")

	// --- Phase 2: Auto-register local repos ---
	// Any local directory not already in the registry gets registered automatically.
	// This replaces the old "ORPHAN" reporting with actionable registration.
	registeredCount := 0
	for _, local := range localRepos {
		// Check if this local path already exists in the registry.
		if e.findRepoByLocalPath(local.Path) != nil {
			continue
		}

		// Auto-register this newly discovered local repo.
		// Use the directory name as the ID since it's the most human-readable.
		newRepo := registry.Repo{}
		newRepo.ID = local.Name
		newRepo.Local.Path = local.Path
		e.reg.Repos = append(e.reg.Repos, newRepo)
		registeredCount++

		r.Add(fmt.Sprintf("REGISTERED: %s → %s", local.Name, local.Path))
	}

	if registeredCount > 0 {
		r.Add(fmt.Sprintf("\nAuto-registered %d new local repo(s).", registeredCount))
	}
	r.Add("")

	// --- Phase 3: Enrich registry with GitHub data ---
	// Cross-link registry entries with GitHub repos by matching names.
	// This fills in GitHub.Name and GitHub.URL for repos that exist on GitHub.
	enrichedGitHubCount := 0
	for _, ghRepo := range githubRepos {
		// Find a matching registry entry by name comparison.
		// Try exact match on ID, then fuzzy match on aliases.
		matchedRepo := e.findRepoByNameMatch(ghRepo.Name)
		if matchedRepo == nil {
			// No local match — register as a GitHub-only repo.
			newRepo := registry.Repo{}
			newRepo.ID = ghRepo.Name
			newRepo.GitHub.Name = ghRepo.Name
			newRepo.GitHub.URL = ghRepo.URL
			e.reg.Repos = append(e.reg.Repos, newRepo)
			r.Add(fmt.Sprintf("GITHUB-ONLY: %s (no local clone)", ghRepo.Name))
			continue
		}

		// Enrich the existing registry entry with GitHub metadata.
		// Only update if the GitHub fields are currently empty.
		if matchedRepo.GitHub.Name == "" {
			matchedRepo.GitHub.Name = ghRepo.Name
			matchedRepo.GitHub.URL = ghRepo.URL
			enrichedGitHubCount++
		}
	}

	if enrichedGitHubCount > 0 {
		r.Add(fmt.Sprintf("Enriched %d repo(s) with GitHub metadata.", enrichedGitHubCount))
	}

	// --- Phase 3b: Enrich registry with ChatProjects data ---
	// Cross-link registry entries with ChatGPT project data.
	enrichedChatCount := 0
	for _, chatProject := range chatProjects {
		// Find a matching registry entry by project name.
		matchedRepo := e.findRepoByNameMatch(chatProject.Name)
		if matchedRepo == nil {
			// No local match — register as a Chat-only entry.
			newRepo := registry.Repo{}
			newRepo.ID = chatProject.Name
			newRepo.Chat.ProjectName = chatProject.Name
			newRepo.Chat.ProjectID = chatProject.ID
			e.reg.Repos = append(e.reg.Repos, newRepo)
			r.Add(fmt.Sprintf("CHAT-ONLY: %s (no local clone)", chatProject.Name))
			continue
		}

		// Enrich the existing registry entry with ChatProjects metadata.
		if matchedRepo.Chat.ProjectID == "" {
			matchedRepo.Chat.ProjectName = chatProject.Name
			matchedRepo.Chat.ProjectID = chatProject.ID
			enrichedChatCount++
		}
	}

	if enrichedChatCount > 0 {
		r.Add(fmt.Sprintf("Enriched %d repo(s) with ChatProjects metadata.", enrichedChatCount))
	}

	// --- Summary ---
	r.Add("")
	r.Add(fmt.Sprintf("Registry total: %d repo(s)", len(e.reg.Repos)))

	return r, nil
}

// findRepoByLocalPath returns a pointer to the registry Repo with the given local
// path, or nil if no match exists. This enables callers to check and mutate the entry.
func (e *Engine) findRepoByLocalPath(localPath string) *registry.Repo {
	for i := range e.reg.Repos {
		if e.reg.Repos[i].Local.Path == localPath {
			return &e.reg.Repos[i]
		}
	}
	return nil
}

// findRepoByNameMatch attempts to match a source name against registry entries
// using progressively looser matching strategies:
//  1. Exact match on Repo.ID
//  2. Case-insensitive match on Repo.ID
//  3. Match on local directory basename
//  4. Match on any alias name
//
// Returns a pointer to the first matching Repo, or nil if no match is found.
func (e *Engine) findRepoByNameMatch(sourceName string) *registry.Repo {
	lowerSourceName := strings.ToLower(sourceName)

	for i := range e.reg.Repos {
		repo := &e.reg.Repos[i]

		// Strategy 1: Exact match on ID.
		if repo.ID == sourceName {
			return repo
		}

		// Strategy 2: Case-insensitive match on ID.
		if strings.ToLower(repo.ID) == lowerSourceName {
			return repo
		}

		// Strategy 3: Match against the local directory basename.
		// Handles cases like local path "/home/me/Code/Krax" matching GitHub "Krax".
		if repo.Local.Path != "" {
			localDirName := filepath.Base(repo.Local.Path)
			if strings.ToLower(localDirName) == lowerSourceName {
				return repo
			}
		}

		// Strategy 4: Match against any registered alias name.
		for _, aliasName := range repo.Aliases.Names {
			if strings.ToLower(aliasName) == lowerSourceName {
				return repo
			}
		}
	}

	// No match found across any strategy.
	return nil
}
