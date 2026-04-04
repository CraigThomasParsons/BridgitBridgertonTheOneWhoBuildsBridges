// Package sync — project.go implements Phase 6: Artifact Projection.
//
// Projection copies specific AAMF artifacts from Bridgit's runtime archive
// into the appropriate repository's docs/ folder. This is a one-way,
// idempotent, non-destructive sync — files are written only if the
// destination doesn't already exist (unless the rule allows overwriting).

package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"

	"bridgit/internal/registry"
)

// ProjectionResult describes the outcome of projecting a single artifact
// from the archive into a target repository. Each result captures the full
// context needed for operator review.
type ProjectionResult struct {
	// JobID is the archive job identifier from the manifest.
	JobID string

	// RepoID is the registry repo this artifact was targeted at.
	RepoID string

	// SourceFile is the absolute path to the artifact in the archive.
	SourceFile string

	// DestFile is the absolute path where the artifact was (or would be) written.
	DestFile string

	// Action describes what happened: COPIED, SKIPPED_EXISTS, SKIPPED_NO_LOCAL,
	// SKIPPED_NO_RULE, SKIPPED_UNREGISTERED, or FAILED.
	Action string

	// Reason provides a human-readable explanation of the action taken.
	Reason string
}

// jobManifest represents the TOML metadata file that links an archive job
// to a specific registry repository. Each job directory in the archive
// must contain a manifest.toml for projection to proceed.
type jobManifest struct {
	Job struct {
		// ID is the unique identifier for this archive job.
		ID string `toml:"id"`

		// RepoID links this job's artifacts to a specific registry entry.
		RepoID string `toml:"repo_id"`

		// Source identifies which bridge produced these artifacts.
		Source string `toml:"source"`
	} `toml:"job"`
}

// ProjectArtifacts scans the runtime archive for completed jobs and copies
// matching artifacts into their target repository's docs/ folder.
//
// Returns a slice of ProjectionResult describing what was copied, skipped,
// or failed. Does not delete source files — the archive is append-only.
// An empty or missing archive directory results in zero results (not an error).
func ProjectArtifacts(
	archivePath string,
	reg *registry.Registry,
	rules []ArtifactRule,
) []ProjectionResult {
	projectionResults := []ProjectionResult{}

	// Check if the archive directory exists before scanning.
	// A missing archive is normal when no bridges have run yet.
	archiveEntries, readError := os.ReadDir(archivePath)
	if readError != nil {
		// Archive doesn't exist or can't be read — nothing to project.
		return projectionResults
	}

	// Each subdirectory in the archive represents a completed bridge job.
	for _, archiveEntry := range archiveEntries {
		// Skip regular files at the archive root — only directories are jobs.
		if !archiveEntry.IsDir() {
			continue
		}

		// Process each job directory and collect its projection results.
		jobPath := filepath.Join(archivePath, archiveEntry.Name())
		jobResults := projectSingleJob(jobPath, reg, rules)
		projectionResults = append(projectionResults, jobResults...)
	}

	return projectionResults
}

// projectSingleJob processes one archive job directory, reading its manifest
// and projecting any matching artifacts into the target repository.
func projectSingleJob(
	jobPath string,
	reg *registry.Registry,
	rules []ArtifactRule,
) []ProjectionResult {
	jobResults := []ProjectionResult{}

	// Read and parse the job manifest to determine the target repo.
	manifest, manifestError := readJobManifest(jobPath)
	if manifestError != nil {
		// Without a manifest, we can't determine where artifacts belong.
		jobResults = append(jobResults, ProjectionResult{
			JobID:  filepath.Base(jobPath),
			Action: "FAILED",
			Reason: fmt.Sprintf("failed to read manifest: %v", manifestError),
		})
		return jobResults
	}

	// Look up the target repo in the registry by the manifest's repo_id.
	targetRepo := findRepoByID(reg, manifest.Job.RepoID)
	if targetRepo == nil {
		// The manifest references a repo that isn't in our registry.
		jobResults = append(jobResults, ProjectionResult{
			JobID:  manifest.Job.ID,
			RepoID: manifest.Job.RepoID,
			Action: "SKIPPED_UNREGISTERED",
			Reason: fmt.Sprintf("repo_id %q not found in registry", manifest.Job.RepoID),
		})
		return jobResults
	}

	// Verify the target repo has a valid local path to project into.
	if targetRepo.Local.Path == "" {
		jobResults = append(jobResults, ProjectionResult{
			JobID:  manifest.Job.ID,
			RepoID: manifest.Job.RepoID,
			Action: "SKIPPED_NO_LOCAL",
			Reason: "registry entry has no local path",
		})
		return jobResults
	}

	// Verify the local path actually exists on disk before attempting writes.
	// Reconciliation (Phase 5) already flags MISSING_LOCAL, but we double-check
	// here to avoid confusing filesystem errors during copy.
	if _, statError := os.Stat(targetRepo.Local.Path); statError != nil {
		jobResults = append(jobResults, ProjectionResult{
			JobID:  manifest.Job.ID,
			RepoID: manifest.Job.RepoID,
			Action: "SKIPPED_NO_LOCAL",
			Reason: fmt.Sprintf("local path does not exist: %s", targetRepo.Local.Path),
		})
		return jobResults
	}

	// Scan the job directory for artifact files to project.
	jobEntries, readError := os.ReadDir(jobPath)
	if readError != nil {
		jobResults = append(jobResults, ProjectionResult{
			JobID:  manifest.Job.ID,
			RepoID: manifest.Job.RepoID,
			Action: "FAILED",
			Reason: fmt.Sprintf("failed to read job directory: %v", readError),
		})
		return jobResults
	}

	// Evaluate each file in the job against the artifact rules.
	for _, jobEntry := range jobEntries {
		// Skip directories and the manifest itself — only artifacts are projected.
		if jobEntry.IsDir() || strings.EqualFold(jobEntry.Name(), "manifest.toml") {
			continue
		}

		// Check if any artifact rule matches this file.
		matchedRule := findMatchingRule(jobEntry.Name(), rules)
		if matchedRule == nil {
			// No rule matches — this file type is not eligible for projection.
			jobResults = append(jobResults, ProjectionResult{
				JobID:      manifest.Job.ID,
				RepoID:     manifest.Job.RepoID,
				SourceFile: filepath.Join(jobPath, jobEntry.Name()),
				Action:     "SKIPPED_NO_RULE",
				Reason:     fmt.Sprintf("no artifact rule matches %s", jobEntry.Name()),
			})
			continue
		}

		// Compute the destination path using the rule's subdir and filename.
		destinationFilename := matchedRule.DestFilename
		if destinationFilename == "" {
			// Preserve the original filename when the rule doesn't specify a rename.
			destinationFilename = jobEntry.Name()
		}
		destinationPath := filepath.Join(targetRepo.Local.Path, matchedRule.DestSubdir, destinationFilename)
		sourcePath := filepath.Join(jobPath, jobEntry.Name())

		// Check if the destination already exists and whether overwriting is allowed.
		if _, statError := os.Stat(destinationPath); statError == nil && !matchedRule.Overwrite {
			// File exists and the rule says don't overwrite — respect existing content.
			jobResults = append(jobResults, ProjectionResult{
				JobID:      manifest.Job.ID,
				RepoID:     manifest.Job.RepoID,
				SourceFile: sourcePath,
				DestFile:   destinationPath,
				Action:     "SKIPPED_EXISTS",
				Reason:     "destination already exists and overwrite is disabled",
			})
			continue
		}

		// Ensure the destination directory exists before writing.
		destinationDir := filepath.Dir(destinationPath)
		if mkdirError := os.MkdirAll(destinationDir, 0755); mkdirError != nil {
			jobResults = append(jobResults, ProjectionResult{
				JobID:      manifest.Job.ID,
				RepoID:     manifest.Job.RepoID,
				SourceFile: sourcePath,
				DestFile:   destinationPath,
				Action:     "FAILED",
				Reason:     fmt.Sprintf("failed to create directory %s: %v", destinationDir, mkdirError),
			})
			continue
		}

		// Copy the artifact from the archive into the target repo.
		copyError := copyFile(sourcePath, destinationPath)
		if copyError != nil {
			jobResults = append(jobResults, ProjectionResult{
				JobID:      manifest.Job.ID,
				RepoID:     manifest.Job.RepoID,
				SourceFile: sourcePath,
				DestFile:   destinationPath,
				Action:     "FAILED",
				Reason:     fmt.Sprintf("copy failed: %v", copyError),
			})
			continue
		}

		// Artifact was successfully projected into the target repo.
		jobResults = append(jobResults, ProjectionResult{
			JobID:      manifest.Job.ID,
			RepoID:     manifest.Job.RepoID,
			SourceFile: sourcePath,
			DestFile:   destinationPath,
			Action:     "COPIED",
			Reason:     "artifact projected successfully",
		})
	}

	return jobResults
}

// readJobManifest parses the manifest.toml file from an archive job directory.
//
// The manifest links the job's artifacts to a specific registry repo via repo_id.
// Returns an error if the manifest is missing or contains invalid TOML.
func readJobManifest(jobPath string) (*jobManifest, error) {
	manifestPath := filepath.Join(jobPath, "manifest.toml")

	// Read the entire manifest file into memory. These files are small
	// (typically under 200 bytes), so buffered reading is unnecessary.
	manifestBytes, readError := os.ReadFile(manifestPath)
	if readError != nil {
		return nil, fmt.Errorf("failed to read manifest at %s: %w", manifestPath, readError)
	}

	// Parse the TOML content into the manifest struct.
	var manifest jobManifest
	if unmarshalError := toml.Unmarshal(manifestBytes, &manifest); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse manifest at %s: %w", manifestPath, unmarshalError)
	}

	// Validate that the manifest has the minimum required fields.
	if manifest.Job.RepoID == "" {
		return nil, fmt.Errorf("manifest at %s is missing required repo_id field", manifestPath)
	}

	return &manifest, nil
}

// copyFile copies a single file from sourcePath to destinationPath.
//
// The destination file is created with 0644 permissions. If the destination
// already exists, it is overwritten (the caller is responsible for checking
// the overwrite policy before calling this function).
func copyFile(sourcePath string, destinationPath string) error {
	// Open the source file for reading.
	sourceFile, openError := os.Open(sourcePath)
	if openError != nil {
		return fmt.Errorf("failed to open source %s: %w", sourcePath, openError)
	}
	defer sourceFile.Close()

	// Create the destination file for writing.
	destinationFile, createError := os.Create(destinationPath)
	if createError != nil {
		return fmt.Errorf("failed to create destination %s: %w", destinationPath, createError)
	}
	defer destinationFile.Close()

	// Stream the contents from source to destination.
	// io.Copy handles buffering internally for efficient transfer.
	if _, copyError := io.Copy(destinationFile, sourceFile); copyError != nil {
		return fmt.Errorf("failed to copy %s → %s: %w", sourcePath, destinationPath, copyError)
	}

	return nil
}

// findRepoByID searches the registry for a repo with the given ID.
//
// Uses case-insensitive comparison because registry IDs are normalized
// to lowercase, but manifests may use mixed case.
func findRepoByID(reg *registry.Registry, repoID string) *registry.Repo {
	lowerRepoID := strings.ToLower(repoID)

	for i := range reg.Repos {
		if strings.ToLower(reg.Repos[i].ID) == lowerRepoID {
			return &reg.Repos[i]
		}
	}

	// No registry entry matches the given repo ID.
	return nil
}
