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
// This function performs five phases:
//  1. Fetch: Pull data from ChatGPT projects, GitHub API, and local filesystem
//  2. Adopt: Discover unregistered local repos and optionally add to registry
//  3. Resolve: Run identity matching (git remote → alias → normalized name)
//  4. Enrich: Cross-link registry entries with GitHub and ChatProjects data
//  5. Report: Accumulate findings for operator review
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

	// --- Phase 2: Discover and adopt unregistered local repos ---
	// Scans local directories, filters noise (dotfiles), builds candidates,
	// and either reports them (preview mode) or persists them (AutoAdopt).
	adoptedCount := 0
	candidateCount := 0

	for _, local := range localRepos {
		// Skip system directories and dotfiles — they aren't real projects.
		if isIgnored(local.Name) {
			continue
		}

		// Check if this local path already has a registry entry.
		if e.findRepoByLocalPath(local.Path) != nil {
			continue
		}

		// Build a structured candidate from the raw scan result.
		candidate := BuildCandidate(local)
		candidateCount++

		// Always report the discovery so operators can review.
		r.Add(fmt.Sprintf("UNREGISTERED LOCAL: %s", local.Path))
		r.Add(fmt.Sprintf("→ suggested id: %s", candidate.ID))

		// Only mutate the registry when AutoAdopt is explicitly enabled.
		// This keeps the default run safe and side-effect free.
		if e.cfg.AutoAdopt {
			// Guard against duplicate IDs from repeated runs.
			if existsInRegistry(e.reg, candidate.ID) {
				r.Add("⚠ skipped (ID already exists in registry)")
				continue
			}

			AddToRegistry(e.reg, candidate)
			adoptedCount++
			r.Add("✓ adopted into registry")
		}
	}

	// Summarize adoption results for quick operator feedback.
	if candidateCount > 0 && !e.cfg.AutoAdopt {
		r.Add(fmt.Sprintf("\nFound %d unregistered repo(s). Set AutoAdopt=true to adopt.", candidateCount))
	}
	if adoptedCount > 0 {
		r.Add(fmt.Sprintf("\nAdopted %d new repo(s) into registry.", adoptedCount))
	}
	r.Add("")

	// --- Phase 3: Identity Resolution ---
	// Run the matching pipeline across all local repos to link them with
	// GitHub repos and registry entries. This uses git remotes as the
	// highest-confidence signal, falling back to aliases and normalized names.
	matchResults := MatchAll(localRepos, e.reg, githubRepos)

	// Counters for the match summary line.
	matchCountByStrategy := map[string]int{
		"git":        0,
		"alias":      0,
		"normalized": 0,
		"none":       0,
	}

	for _, match := range matchResults {
		matchCountByStrategy[match.Strategy]++

		switch match.Strategy {
		case "git":
			r.Add(fmt.Sprintf("MATCHED (git): %s → %s", match.LocalName, match.RepoID))

			// Use the git remote to enrich the registry entry with GitHub data.
			// This is the most reliable link — the repo itself declares its origin.
			e.enrichFromGitMatch(match, githubRepos)

		case "alias":
			r.Add(fmt.Sprintf("MATCHED (alias): %s → %s", match.LocalName, match.RepoID))

		case "normalized":
			r.Add(fmt.Sprintf("MATCHED (normalized): %s → %s", match.LocalName, match.RepoID))

		case "none":
			r.Add(fmt.Sprintf("UNRESOLVED: %s", match.LocalName))
		}
	}

	// Print a one-line summary of match distribution.
	r.Add(fmt.Sprintf(
		"\nIdentity resolution: %d git, %d alias, %d normalized, %d unresolved",
		matchCountByStrategy["git"],
		matchCountByStrategy["alias"],
		matchCountByStrategy["normalized"],
		matchCountByStrategy["none"],
	))
	r.Add("")

	// --- Phase 4: Enrich registry with remaining GitHub data ---
	// Cross-link registry entries with GitHub repos that weren't already
	// linked by the git remote matcher. Uses name-based fallback matching.
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

	// --- Phase 4b: Enrich registry with ChatProjects data ---
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

// enrichFromGitMatch updates a registry entry with GitHub metadata discovered
// via the git remote. This is the highest-confidence enrichment path because
// the repo's own .git/config declares its GitHub identity.
//
// If the matched repo ID corresponds to a registry entry, its GitHub fields
// are filled in from the API data. If no registry entry exists yet, a new
// one is created so the GitHub link isn't lost.
func (e *Engine) enrichFromGitMatch(match MatchResult, githubRepos []GitHubRepo) {
	// Find the GitHub API entry that matches the git remote name.
	var matchedGitHub *GitHubRepo
	for i := range githubRepos {
		if strings.EqualFold(githubRepos[i].Name, match.GitHubName) {
			matchedGitHub = &githubRepos[i]
			break
		}
	}

	// If the GitHub API doesn't know this repo, we can't enrich further.
	// The match itself is still valid — just no URL to fill in.
	if matchedGitHub == nil {
		return
	}

	// Find the registry entry to enrich. Try by local path first (most precise),
	// then fall back to the matched repo ID.
	registryEntry := e.findRepoByLocalPath(match.LocalPath)
	if registryEntry == nil {
		registryEntry = e.findRepoByNameMatch(match.RepoID)
	}

	// If no registry entry exists, nothing to enrich.
	// Phase 2 adoption should have created it already.
	if registryEntry == nil {
		return
	}

	// Fill in GitHub metadata only if not already set.
	// Avoids overwriting data from a previous, possibly more complete, enrichment.
	if registryEntry.GitHub.Name == "" {
		registryEntry.GitHub.Name = matchedGitHub.Name
		registryEntry.GitHub.URL = matchedGitHub.URL
	}
}
