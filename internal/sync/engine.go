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
// This function performs six phases:
//  1. Fetch: Pull data from ChatGPT projects, GitHub API, and local filesystem
//  2. Adopt: Discover unregistered local repos and optionally add to registry
//  3. Resolve: Run identity matching (git remote → alias → normalized name)
//  4. Enrich: Cross-link registry entries with GitHub and ChatProjects data
//  5. Reconcile: Detect drift between registry, GitHub, and local filesystem
//  6. Report: Accumulate findings for operator review
//
// Errors from individual sources are logged as warnings in the report,
// allowing other sources to proceed even if one fails.
func (engine *Engine) Run() (*report.Report, error) {
	// Initialize a fresh report to accumulate findings.
	syncReport := report.New()

	// Add the report header for visual clarity in stdout.
	syncReport.Add("== Bridgit Sync Report ==")
	syncReport.Add("")

	// --- Phase 1: Fetch from all three sources ---
	// Each source is fetched independently; failures are reported but non-fatal.

	// Fetch ChatGPT projects from the bridge outbox.
	chatProjects, chatError := FetchChatProjects()
	if chatError != nil {
		syncReport.Add(fmt.Sprintf("WARNING: Chat Projects fetch failed: %v", chatError))
	}

	// Fetch GitHub repos via the REST API.
	githubRepos, githubError := FetchGitHubRepos(engine.cfg.GitHubOwner)
	if githubError != nil {
		syncReport.Add(fmt.Sprintf("WARNING: GitHub fetch failed: %v", githubError))
	}

	// Scan local directories under CodeRoot.
	localRepos, localError := ScanLocal(engine.cfg.CodeRoot)
	if localError != nil {
		// Local scan failure is more serious since it's purely filesystem.
		return nil, fmt.Errorf("failed to scan local repos at %s: %w", engine.cfg.CodeRoot, localError)
	}

	// Report the count of repositories discovered in each source.
	// Provides quick feedback on whether sources are responding.
	syncReport.Add(fmt.Sprintf("Chat Projects: %d", len(chatProjects)))
	syncReport.Add(fmt.Sprintf("GitHub Repos:  %d", len(githubRepos)))
	syncReport.Add(fmt.Sprintf("Local Repos:   %d", len(localRepos)))
	syncReport.Add("")

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
		if engine.findRepoByLocalPath(local.Path) != nil {
			continue
		}

		// Build a structured candidate from the raw scan result.
		candidate := BuildCandidate(local)
		candidateCount++

		// Always report the discovery so operators can review.
		syncReport.Add(fmt.Sprintf("UNREGISTERED LOCAL: %s", local.Path))
		syncReport.Add(fmt.Sprintf("→ suggested id: %s", candidate.ID))

		// Only mutate the registry when AutoAdopt is explicitly enabled.
		// This keeps the default run safe and side-effect free.
		if engine.cfg.AutoAdopt {
			// Guard against duplicate IDs from repeated runs.
			if existsInRegistry(engine.reg, candidate.ID) {
				syncReport.Add("⚠ skipped (ID already exists in registry)")
				continue
			}

			AddToRegistry(engine.reg, candidate)
			adoptedCount++
			syncReport.Add("✓ adopted into registry")
		}
	}

	// Summarize adoption results for quick operator feedback.
	if candidateCount > 0 && !engine.cfg.AutoAdopt {
		syncReport.Add(fmt.Sprintf("\nFound %d unregistered repo(s). Set AutoAdopt=true to adopt.", candidateCount))
	}
	if adoptedCount > 0 {
		syncReport.Add(fmt.Sprintf("\nAdopted %d new repo(s) into registry.", adoptedCount))
	}
	syncReport.Add("")

	// --- Phase 3: Identity Resolution ---
	// Run the matching pipeline across all local repos to link them with
	// GitHub repos and registry entries. This uses git remotes as the
	// highest-confidence signal, falling back to aliases and normalized names.
	matchResults := MatchAll(localRepos, engine.reg, githubRepos)

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
			syncReport.Add(fmt.Sprintf("MATCHED (git): %s → %s", match.LocalName, match.RepoID))

			// Use the git remote to enrich the registry entry with GitHub data.
			// This is the most reliable link — the repo itself declares its origin.
			engine.enrichFromGitMatch(match, githubRepos)

		case "alias":
			syncReport.Add(fmt.Sprintf("MATCHED (alias): %s → %s", match.LocalName, match.RepoID))

		case "normalized":
			syncReport.Add(fmt.Sprintf("MATCHED (normalized): %s → %s", match.LocalName, match.RepoID))

		case "none":
			syncReport.Add(fmt.Sprintf("UNRESOLVED: %s", match.LocalName))
		}
	}

	// Print a one-line summary of match distribution.
	syncReport.Add(fmt.Sprintf(
		"\nIdentity resolution: %d git, %d alias, %d normalized, %d unresolved",
		matchCountByStrategy["git"],
		matchCountByStrategy["alias"],
		matchCountByStrategy["normalized"],
		matchCountByStrategy["none"],
	))
	syncReport.Add("")

	// --- Phase 4: Enrich registry with remaining GitHub data ---
	// Cross-link registry entries with GitHub repos that weren't already
	// linked by the git remote matcher. Uses name-based fallback matching.
	enrichedGitHubCount := 0
	for _, ghRepo := range githubRepos {
		// Find a matching registry entry by name comparison.
		// Try exact match on ID, then fuzzy match on aliases.
		matchedRepo := engine.findRepoByNameMatch(ghRepo.Name)
		if matchedRepo == nil {
			// No local match — register as a GitHub-only repo.
			newRepo := registry.Repo{}
			newRepo.ID = ghRepo.Name
			newRepo.GitHub.Name = ghRepo.Name
			newRepo.GitHub.URL = ghRepo.URL
			engine.reg.Repos = append(engine.reg.Repos, newRepo)
			syncReport.Add(fmt.Sprintf("GITHUB-ONLY: %s (no local clone)", ghRepo.Name))
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
		syncReport.Add(fmt.Sprintf("Enriched %d repo(s) with GitHub metadata.", enrichedGitHubCount))
	}

	// --- Phase 4b: Enrich registry with ChatProjects data ---
	// Cross-link registry entries with ChatGPT project data.
	enrichedChatCount := 0
	for _, chatProject := range chatProjects {
		// Find a matching registry entry by project name.
		matchedRepo := engine.findRepoByNameMatch(chatProject.Name)
		if matchedRepo == nil {
			// No local match — register as a Chat-only entry.
			newRepo := registry.Repo{}
			newRepo.ID = chatProject.Name
			newRepo.Chat.ProjectName = chatProject.Name
			newRepo.Chat.ProjectID = chatProject.ID
			engine.reg.Repos = append(engine.reg.Repos, newRepo)
			syncReport.Add(fmt.Sprintf("CHAT-ONLY: %s (no local clone)", chatProject.Name))
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
		syncReport.Add(fmt.Sprintf("Enriched %d repo(s) with ChatProjects metadata.", enrichedChatCount))
	}

	// --- Phase 5: Reconciliation ---
	// Compare registry state against GitHub and local sources to detect
	// misalignment. This is read-only — no mutations, only diagnostics.
	reconcileResults := Reconcile(engine.reg, githubRepos, localRepos)

	syncReport.Add("")
	syncReport.Add("== Reconciliation Report ==")
	syncReport.Add("")

	// Separate OK results from issues so operators see problems first.
	okCount := 0
	for _, reconcileResult := range reconcileResults {
		switch reconcileResult.Status {
		case "OK":
			// Healthy repos are counted but not printed individually.
			okCount++
		default:
			// Print each issue with its detail lines for operator triage.
			syncReport.Add(fmt.Sprintf("%s: %s", reconcileResult.Status, reconcileResult.RepoID))
			for _, detailLine := range reconcileResult.Details {
				syncReport.Add(fmt.Sprintf("  → %s", detailLine))
			}
		}
	}

	// Summary line gives operators a quick pass/fail signal.
	issueCount := len(reconcileResults) - okCount
	syncReport.Add(fmt.Sprintf("\n%d OK, %d issues detected", okCount, issueCount))

	// --- Summary ---
	syncReport.Add("")
	syncReport.Add(fmt.Sprintf("Registry total: %d repo(s)", len(engine.reg.Repos)))

	return syncReport, nil
}

// findRepoByLocalPath returns a pointer to the registry Repo with the given local
// path, or nil if no match exists. This enables callers to check and mutate the entry.
func (engine *Engine) findRepoByLocalPath(localPath string) *registry.Repo {
	for i := range engine.reg.Repos {
		if engine.reg.Repos[i].Local.Path == localPath {
			return &engine.reg.Repos[i]
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
func (engine *Engine) findRepoByNameMatch(sourceName string) *registry.Repo {
	lowerSourceName := strings.ToLower(sourceName)

	for i := range engine.reg.Repos {
		repo := &engine.reg.Repos[i]

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
func (engine *Engine) enrichFromGitMatch(match MatchResult, githubRepos []GitHubRepo) {
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
	registryEntry := engine.findRepoByLocalPath(match.LocalPath)
	if registryEntry == nil {
		registryEntry = engine.findRepoByNameMatch(match.RepoID)
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
