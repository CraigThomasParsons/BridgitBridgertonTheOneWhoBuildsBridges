// Package contracts — events.go defines the event types and Event struct
// for Bridgit's sync lifecycle event system.
//
// Events represent meaningful state transitions during sync execution.
// Each event captures what happened, which phase emitted it, and optional
// metadata for downstream subscribers to act on.

package contracts

import "time"

// EventType is a typed string representing a category of sync lifecycle event.
// Using a string type instead of iota allows human-readable log output
// without needing a separate stringer or lookup table.
type EventType string

const (
	// PhaseStarted fires when an engine phase begins execution.
	PhaseStarted EventType = "phase.started"

	// PhaseCompleted fires when an engine phase finishes successfully.
	PhaseCompleted EventType = "phase.completed"

	// PackageReceived fires when the intake phase discovers a package in the inbox.
	PackageReceived EventType = "package.received"

	// ManifestGenerated fires after successfully transforming letter.toml into manifest.toml.
	ManifestGenerated EventType = "manifest.generated"

	// PackageArchived fires when a processed package is moved to the archive.
	PackageArchived EventType = "package.archived"

	// PackageFailed fires when intake processing fails for a package.
	// The package is moved to runtime/failed/ for operator inspection.
	PackageFailed EventType = "package.failed"

	// RegistryLookupFailed fires when a letter.toml project_id cannot be resolved
	// to a registry repo_id. This typically means the registry hasn't been
	// enriched with ChatProjects data yet.
	RegistryLookupFailed EventType = "registry.lookup_failed"

	// ArtifactProjected fires when an artifact is successfully copied
	// from the archive into a target repository's docs/ folder.
	ArtifactProjected EventType = "artifact.projected"

	// ArtifactSkipped fires when an artifact projection is skipped
	// because the destination already exists or no rule matches.
	ArtifactSkipped EventType = "artifact.skipped"
)

// Event captures a single lifecycle occurrence during sync execution.
//
// Events are emitted by engine phases and consumed by subscribers (report writer,
// log file, future webhook triggers). The struct is intentionally flat and
// serializable to support both in-memory and persistent consumption.
type Event struct {
	// Type categorizes what happened (e.g., phase.started, package.archived).
	Type EventType

	// Timestamp records when the event was emitted. Set by the emitter,
	// not by the caller, to ensure consistent clock source.
	Timestamp time.Time

	// Phase identifies which engine phase emitted this event (e.g., "fetch", "intake").
	Phase string

	// RepoID is the registry repo ID associated with the event, if applicable.
	// Empty for phase-level events that don't target a specific repo.
	RepoID string

	// Message is a human-readable description of what happened.
	// Should be a complete sentence suitable for log output.
	Message string

	// Metadata holds arbitrary key-value context for downstream consumers.
	// Avoids bloating the struct with fields for every possible scenario.
	// Example: {"project_id": "42", "source": "ChatProjectsToKraxBridge"}
	Metadata map[string]string
}
