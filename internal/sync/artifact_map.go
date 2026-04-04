// Package sync — artifact_map.go defines the projection rules that control
// which archive artifacts are eligible for projection into repository docs.
//
// Rules are hardcoded for now. A future iteration may load them from a
// TOML config file to allow per-project customization.

package sync

// ArtifactRule defines where a specific artifact type should be projected.
//
// Each rule maps a source filename pattern in the archive to a destination
// path under the target repository's root. Rules are evaluated in order,
// and the first matching rule wins for each source file.
type ArtifactRule struct {
	// SourcePattern is the exact filename to match (e.g., "VISION.md").
	// Glob support may be added later, but exact match is sufficient for now.
	SourcePattern string

	// DestSubdir is the subdirectory under the repo root where the artifact
	// should be placed (e.g., "docs/architecture"). Created if missing.
	DestSubdir string

	// DestFilename is the target filename. If empty, the original source
	// filename is preserved as-is.
	DestFilename string

	// Overwrite controls whether an existing destination file should be replaced.
	// Default false ensures projection never destroys hand-written content.
	Overwrite bool
}

// DefaultArtifactRules returns the hardcoded set of projection rules for
// known AAMF artifact types.
//
// These rules define a one-way sync from the archive into each repo's docs
// folder. All rules default to Overwrite: false to prevent clobbering
// hand-maintained documentation.
func DefaultArtifactRules() []ArtifactRule {
	return []ArtifactRule{
		{
			// Vision documents describe the project's purpose and direction.
			SourcePattern: "VISION.md",
			DestSubdir:    "docs/architecture",
			DestFilename:  "vision.md",
			Overwrite:     false,
		},
		{
			// Persona documents define the target users and their goals.
			SourcePattern: "PERSONAS.md",
			DestSubdir:    "docs/architecture",
			DestFilename:  "personas.md",
			Overwrite:     false,
		},
		{
			// Decision logs capture key architectural choices and trade-offs.
			SourcePattern: "DECISIONS.md",
			DestSubdir:    "docs/decisions",
			DestFilename:  "decisions.md",
			Overwrite:     false,
		},
		{
			// README is projected to repo root but never overwrites — repos
			// typically already have a README with custom content.
			SourcePattern: "README.md",
			DestSubdir:    "",
			DestFilename:  "README.md",
			Overwrite:     false,
		},
		{
			// Stack documents describe the technology choices for the project.
			SourcePattern: "STACK.md",
			DestSubdir:    "docs/architecture",
			DestFilename:  "stack.md",
			Overwrite:     false,
		},
		{
			// Roadmap documents outline planned features and milestones.
			SourcePattern: "ROADMAP.md",
			DestSubdir:    "docs",
			DestFilename:  "roadmap.md",
			Overwrite:     false,
		},
		{
			// Epic breakdowns organize features into implementation groups.
			// Produced by the ChatProjectsToKraxBridge LLM extraction pipeline.
			SourcePattern: "EPICS.md",
			DestSubdir:    "docs/architecture",
			DestFilename:  "epics.md",
			Overwrite:     false,
		},
		{
			// User stories derived from personas and features by the Krax bridge.
			// These translate high-level goals into actionable development tasks.
			SourcePattern: "STORIES.md",
			DestSubdir:    "docs/architecture",
			DestFilename:  "stories.md",
			Overwrite:     false,
		},
		{
			// Technical and business constraints identified during the inception process.
			// These guard rails prevent scope drift and maintain architectural integrity.
			SourcePattern: "CONSTRAINTS.md",
			DestSubdir:    "docs/architecture",
			DestFilename:  "constraints.md",
			Overwrite:     false,
		},
	}
}

// findMatchingRule returns the first ArtifactRule whose SourcePattern matches
// the given filename, or nil if no rule applies.
//
// This keeps unknown file types out of projection entirely — only explicitly
// mapped artifacts are eligible for copying.
func findMatchingRule(filename string, rules []ArtifactRule) *ArtifactRule {
	for i, rule := range rules {
		// Exact match on filename. Case-sensitive because AAMF artifact
		// naming is a controlled convention (always UPPER.md).
		if rule.SourcePattern == filename {
			return &rules[i]
		}
	}

	// No rule matches — this file type is not eligible for projection.
	return nil
}
