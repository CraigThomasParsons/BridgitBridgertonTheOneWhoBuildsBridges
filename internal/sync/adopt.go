// Package sync — adopt.go implements the Phase 2 adoption flow.
//
// Adoption converts unregistered local directories into structured registry
// entries. The flow is: scan → filter noise → check registry → build
// candidate → optionally persist. This keeps mutation opt-in via AutoAdopt.
package sync

import (
	"strings"

	"bridgit/internal/registry"
)

// Candidate represents a local repo discovered on disk but not yet in the
// registry. It carries a normalized ID, the original directory name, and
// the absolute path — everything needed to create a registry entry.
type Candidate struct {
	// ID is the kebab-case normalized form of the directory name.
	ID string

	// Path is the absolute filesystem path to the repository root.
	Path string

	// Name is the original directory name before normalization.
	Name string
}

// normalizeID converts a directory name into a kebab-case registry ID.
//
// Lowercases the name and replaces spaces and underscores with hyphens.
// This ensures consistent lookup regardless of naming conventions.
func normalizeID(name string) string {
	// Lowercase first to guarantee case-insensitive dedup.
	id := strings.ToLower(name)

	// Replace common separator characters with a single hyphen style.
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// BuildCandidate constructs an adoption Candidate from a local repo scan result.
//
// The candidate's ID is derived from the directory name via normalizeID,
// ensuring consistent kebab-case formatting for registry entries.
func BuildCandidate(local LocalRepo) Candidate {
	return Candidate{
		ID:   normalizeID(local.Name),
		Name: local.Name,
		Path: local.Path,
	}
}

// AddToRegistry appends a Candidate as a new Repo entry in the registry.
//
// The entry is created with the normalized ID, local path, and the original
// directory name stored as an alias for fuzzy matching during enrichment.
func AddToRegistry(reg *registry.Registry, candidate Candidate) {
	// Build a minimal registry entry from the candidate fields.
	repo := registry.Repo{
		ID: candidate.ID,
	}

	// Set the local path so Phase 3 enrichment can cross-link this repo.
	repo.Local.Path = candidate.Path

	// Store the original name as an alias for name-based matching.
	// GitHub and ChatProjects enrichment use aliases for fuzzy lookups.
	repo.Aliases.Names = []string{candidate.Name}

	reg.Repos = append(reg.Repos, repo)
}

// existsInRegistry checks whether a repo with the given ID is already tracked.
//
// This prevents duplicate entries when AutoAdopt runs multiple times on the
// same code root. Uses exact string comparison on the normalized ID field.
func existsInRegistry(reg *registry.Registry, id string) bool {
	for _, repo := range reg.Repos {
		if repo.ID == id {
			return true
		}
	}
	return false
}

// isIgnored returns true for directory names that should be skipped during
// adoption scanning. Dotfiles and system configuration folders are noise —
// they aren't real project repositories.
func isIgnored(name string) bool {
	// All dotfiles (e.g., .git, .config) are system artifacts.
	if strings.HasPrefix(name, ".") {
		return true
	}

	return false
}
