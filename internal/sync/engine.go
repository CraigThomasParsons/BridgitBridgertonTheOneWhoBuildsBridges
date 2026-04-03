// Package sync implements the core synchronization engine for Bridgit.
//
// The engine coordinates fetching from multiple sources (ChatGPT projects,
// GitHub repos, local filesystems) and reconciles them against the registry
// to detect orphaned repos and missing links.
package sync

import (
	"fmt"

	"bridgit/internal/config"
	"bridgit/internal/registry"
	"bridgit/internal/report"
)

// Engine orchestrates multi-source synchronization and reconciliation.
//
// It holds references to configuration and the registry, allowing it to
// fetch from external sources and compare against known state.
type Engine struct {
	// cfg contains runtime configuration like code root and GitHub owner.
	cfg *config.Config

	// reg is the shared registry of known repositories.
	// The engine reads from this to detect orphans but does not mutate it.
	reg *registry.Registry
}

// NewEngine creates a new sync Engine with the given configuration and registry.
//
// The config and registry are captured by pointer to allow the engine to
// inspect current state without copying large structures.
func NewEngine(cfg config.Config, reg *registry.Registry) *Engine {
	// Store pointers to avoid copying and enable shared state access.
	return &Engine{
		cfg: &cfg,
		reg: reg,
	}
}

// Run executes the full synchronization workflow across all sources.
//
// This function fetches from ChatGPT projects, GitHub, and the local filesystem,
// then performs reconciliation checks to identify orphaned local folders.
// The report accumulates all findings for operator review.
//
// NOTE: Error handling is currently permissive (errors are ignored with _).
// Production code should propagate fetch errors or accumulate warnings.
func (e *Engine) Run() (*report.Report, error) {
	// Initialize a fresh report to accumulate findings.
	r := report.New()

	// Add the report header for visual clarity in stdout.
	r.Add("== Bridgit Sync Report ==")

	// Fetch from all three sources concurrently (TODO: add concurrency).
	// Errors are currently ignored (_) but should be handled in production.
	chatProjects, _ := FetchChatProjects()
	githubRepos, _ := FetchGitHubRepos(e.cfg.GitHubOwner)
	localRepos, _ := ScanLocal(e.cfg.CodeRoot)

	// Report the count of repositories discovered in each source.
	// Provides quick feedback on whether sources are responding.
	r.Add(fmt.Sprintf("Chat Projects: %d", len(chatProjects)))
	r.Add(fmt.Sprintf("GitHub Repos: %d", len(githubRepos)))
	r.Add(fmt.Sprintf("Local Repos: %d", len(localRepos)))

	// Reconciliation: Find local folders not tracked in the registry.
	// These are "orphaned" repos that should potentially be registered.
	for _, local := range localRepos {
		// Check if this local path exists in the registry.
		// Linear search is acceptable for small repo counts (<1000).
		found := false

		for _, repo := range e.reg.Repos {
			// Match on exact path to avoid false positives.
			if repo.Local.Path == local.Path {
				found = true
				// Break early once we find a match.
				break
			}
		}

		// Report orphaned local folders for operator review.
		// These may need manual registration or cleanup.
		if !found {
			r.Add("ORPHAN LOCAL: " + local.Path)
		}
	}

	// Return the accumulated report.
	// No errors are currently propagated from this function.
	return r, nil
}
