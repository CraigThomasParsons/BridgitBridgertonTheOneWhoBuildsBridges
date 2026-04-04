// Package sync — intake.go implements the intake phase that bridges the gap
// between the filesystem router's inbox and the projection engine's archive.
//
// Packages arrive in runtime/inbox/ from bridge outboxes via the router.
// The intake phase reads each package's letter.toml, resolves the project_id
// to a registry repo_id, generates a manifest.toml that the projection engine
// understands, and moves the package to runtime/archive/. Failed packages
// are moved to runtime/failed/ for operator inspection.

package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"bridgit/internal/contracts"
	"bridgit/internal/registry"
)

// IntakeResult describes the outcome of processing a single inbox package.
// Each result captures enough context for the report and event log to
// provide actionable feedback to operators.
type IntakeResult struct {
	// PackageName is the directory name of the inbox package (e.g., "project-42").
	PackageName string

	// RepoID is the resolved registry repo ID. Empty on failure.
	RepoID string

	// Action describes what happened: PROCESSED, FAILED_LOOKUP, FAILED_READ,
	// FAILED_MANIFEST, FAILED_MOVE, ALREADY_EXISTS.
	Action string

	// Reason provides a human-readable explanation of the outcome.
	Reason string
}

// letterEnvelope is the TOML deserialization target for letter.toml files
// produced by the ChatProjectsToKraxBridge. The project_id field is an
// integer in the bridge output (TOML integer type, no quotes).
type letterEnvelope struct {
	// Recipient identifies which downstream system the package is intended for.
	Recipient string `toml:"recipient"`

	// ProjectID is the ChatProjects database ID for the source project.
	// This needs to be resolved to a registry repo_id via registry lookup.
	ProjectID int64 `toml:"project_id"`

	// Stage indicates the processing stage of the package (e.g., "extracted").
	Stage string `toml:"stage"`
}

// manifestOutput is the TOML structure written to manifest.toml for the
// projection engine to consume. Matches the format expected by
// readJobManifest() in project.go.
type manifestOutput struct {
	Job manifestJobSection `toml:"job"`
}

// manifestJobSection holds the fields within the [job] TOML table.
type manifestJobSection struct {
	// ID uniquely identifies this archive job across runs.
	ID string `toml:"id"`

	// RepoID links the job's artifacts to a specific registry entry.
	RepoID string `toml:"repo_id"`

	// Source identifies which bridge produced these artifacts.
	Source string `toml:"source"`

	// CreatedAt records when the manifest was generated.
	CreatedAt string `toml:"created_at"`
}

// ProcessInbox scans the runtime inbox for packages, transforms their
// letter.toml metadata into manifest.toml format, and moves them to
// the archive where the projection engine can find them.
//
// Packages that fail processing are moved to the failed directory instead
// of being left in the inbox. This ensures the inbox is always clean and
// operators can find problematic packages in a single location.
func ProcessInbox(
	inboxPath string,
	archivePath string,
	failedPath string,
	reg *registry.Registry,
	emitter contracts.Emitter,
) []IntakeResult {
	intakeResults := []IntakeResult{}

	// Check if the inbox directory exists before scanning.
	// An empty or missing inbox is normal — not all runs produce packages.
	inboxEntries, readError := os.ReadDir(inboxPath)
	if readError != nil {
		// Inbox doesn't exist or can't be read — nothing to process.
		return intakeResults
	}

	// Process each subdirectory in the inbox as a package.
	for _, inboxEntry := range inboxEntries {
		// Skip regular files — only directories are valid packages.
		if !inboxEntry.IsDir() {
			continue
		}

		// Emit a discovery event before processing.
		packagePath := filepath.Join(inboxPath, inboxEntry.Name())
		emitter.Emit(contracts.Event{
			Type:    contracts.PackageReceived,
			Phase:   "intake",
			Message: fmt.Sprintf("discovered package %s in inbox", inboxEntry.Name()),
		})

		// Process the individual package and collect the result.
		result := processSinglePackage(packagePath, archivePath, failedPath, reg, emitter)
		intakeResults = append(intakeResults, result)
	}

	return intakeResults
}

// processSinglePackage handles the full intake workflow for one package:
// read letter.toml, resolve repo_id, generate manifest.toml, move to archive.
func processSinglePackage(
	packagePath string,
	archivePath string,
	failedPath string,
	reg *registry.Registry,
	emitter contracts.Emitter,
) IntakeResult {
	packageName := filepath.Base(packagePath)

	// Step 1: Read and parse the letter.toml from the package.
	letter, letterError := readLetterEnvelope(packagePath)
	if letterError != nil {
		// Move the broken package to failed/ so it doesn't block future runs.
		moveToFailed(packagePath, failedPath)
		emitter.Emit(contracts.Event{
			Type:    contracts.PackageFailed,
			Phase:   "intake",
			Message: fmt.Sprintf("failed to read letter.toml in %s: %v", packageName, letterError),
		})
		return IntakeResult{
			PackageName: packageName,
			Action:      "FAILED_READ",
			Reason:      fmt.Sprintf("failed to read letter.toml: %v", letterError),
		}
	}

	// Step 2: Resolve the numeric project_id to a registry repo_id.
	resolvedRepoID, lookupError := resolveRepoIDByProjectID(reg, letter.ProjectID)
	if lookupError != nil {
		// Move to failed/ — the registry likely needs ChatProjects enrichment first.
		moveToFailed(packagePath, failedPath)
		emitter.Emit(contracts.Event{
			Type:    contracts.RegistryLookupFailed,
			Phase:   "intake",
			RepoID:  fmt.Sprintf("project_id:%d", letter.ProjectID),
			Message: fmt.Sprintf("cannot resolve project_id %d to a registry repo: %v", letter.ProjectID, lookupError),
			Metadata: map[string]string{
				"project_id": strconv.FormatInt(letter.ProjectID, 10),
			},
		})
		return IntakeResult{
			PackageName: packageName,
			Action:      "FAILED_LOOKUP",
			Reason:      fmt.Sprintf("project_id %d not found in registry", letter.ProjectID),
		}
	}

	// Step 3: Check if this package already exists in the archive.
	// Prevents duplicate processing from repeated router runs.
	archiveDestination := filepath.Join(archivePath, packageName)
	if _, statError := os.Stat(archiveDestination); statError == nil {
		return IntakeResult{
			PackageName: packageName,
			RepoID:      resolvedRepoID,
			Action:      "ALREADY_EXISTS",
			Reason:      "package already present in archive",
		}
	}

	// Step 4: Generate manifest.toml inside the package directory.
	// This transforms the letter.toml metadata into the format that
	// the projection engine's readJobManifest() expects.
	jobID := generateJobID(packageName)
	manifestError := generateAndWriteManifest(packagePath, jobID, resolvedRepoID, "ChatProjectsToKraxBridge")
	if manifestError != nil {
		moveToFailed(packagePath, failedPath)
		emitter.Emit(contracts.Event{
			Type:    contracts.PackageFailed,
			Phase:   "intake",
			RepoID:  resolvedRepoID,
			Message: fmt.Sprintf("failed to generate manifest for %s: %v", packageName, manifestError),
		})
		return IntakeResult{
			PackageName: packageName,
			RepoID:      resolvedRepoID,
			Action:      "FAILED_MANIFEST",
			Reason:      fmt.Sprintf("manifest generation failed: %v", manifestError),
		}
	}

	// Step 5: Move the package from inbox to archive.
	// This is the final step — after this, projection can find it.
	moveError := movePackage(packagePath, archiveDestination)
	if moveError != nil {
		emitter.Emit(contracts.Event{
			Type:    contracts.PackageFailed,
			Phase:   "intake",
			RepoID:  resolvedRepoID,
			Message: fmt.Sprintf("failed to move %s to archive: %v", packageName, moveError),
		})
		return IntakeResult{
			PackageName: packageName,
			RepoID:      resolvedRepoID,
			Action:      "FAILED_MOVE",
			Reason:      fmt.Sprintf("move to archive failed: %v", moveError),
		}
	}

	// Package successfully processed and archived.
	emitter.Emit(contracts.Event{
		Type:    contracts.PackageArchived,
		Phase:   "intake",
		RepoID:  resolvedRepoID,
		Message: fmt.Sprintf("package %s archived for repo %s", packageName, resolvedRepoID),
		Metadata: map[string]string{
			"project_id": strconv.FormatInt(letter.ProjectID, 10),
			"job_id":     jobID,
		},
	})

	return IntakeResult{
		PackageName: packageName,
		RepoID:      resolvedRepoID,
		Action:      "PROCESSED",
		Reason:      fmt.Sprintf("archived and ready for projection to repo %s", resolvedRepoID),
	}
}

// readLetterEnvelope parses the letter.toml file from a package directory.
//
// Returns an error if the file is missing, malformed, or missing the
// required project_id field.
func readLetterEnvelope(packagePath string) (*letterEnvelope, error) {
	letterPath := filepath.Join(packagePath, "letter.toml")

	// Read the small metadata file into memory.
	letterBytes, readError := os.ReadFile(letterPath)
	if readError != nil {
		return nil, fmt.Errorf("failed to read letter at %s: %w", letterPath, readError)
	}

	// Parse the TOML content into the envelope struct.
	var letter letterEnvelope
	if unmarshalError := toml.Unmarshal(letterBytes, &letter); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse letter at %s: %w", letterPath, unmarshalError)
	}

	// Validate the minimum required field for intake processing.
	if letter.ProjectID == 0 {
		return nil, fmt.Errorf("letter at %s is missing required project_id field", letterPath)
	}

	return &letter, nil
}

// resolveRepoIDByProjectID searches the registry for a repo whose
// Chat.ProjectID matches the string representation of the given project_id.
//
// The ChatProjects bridge uses integer IDs, but the registry stores them
// as strings (populated during Phase 4 enrichment). This function bridges
// the type mismatch.
func resolveRepoIDByProjectID(reg *registry.Registry, projectID int64) (string, error) {
	// Convert the integer to a string for comparison against registry data.
	projectIDString := strconv.FormatInt(projectID, 10)

	// Linear scan through the registry. Acceptable performance for the
	// typical registry size (~100 entries). No index needed yet.
	for _, registryRepo := range reg.Repos {
		if registryRepo.Chat.ProjectID == projectIDString {
			return registryRepo.ID, nil
		}
	}

	// No registry entry has a matching Chat.ProjectID.
	return "", fmt.Errorf("no registry entry with Chat.ProjectID == %q", projectIDString)
}

// generateAndWriteManifest creates a manifest.toml file inside the package
// directory, transforming bridge-specific letter.toml metadata into the
// standard format that the projection engine expects.
func generateAndWriteManifest(packagePath string, jobID string, repoID string, source string) error {
	// Build the manifest struct matching the format readJobManifest() parses.
	manifest := manifestOutput{
		Job: manifestJobSection{
			ID:        jobID,
			RepoID:    repoID,
			Source:    source,
			CreatedAt: time.Now().Format(time.RFC3339),
		},
	}

	// Marshal the manifest to TOML bytes.
	manifestBytes, marshalError := toml.Marshal(manifest)
	if marshalError != nil {
		return fmt.Errorf("failed to marshal manifest: %w", marshalError)
	}

	// Write manifest.toml into the package directory alongside the artifacts.
	manifestPath := filepath.Join(packagePath, "manifest.toml")
	if writeError := os.WriteFile(manifestPath, manifestBytes, 0644); writeError != nil {
		return fmt.Errorf("failed to write manifest at %s: %w", manifestPath, writeError)
	}

	return nil
}

// generateJobID creates a unique job identifier from the package directory
// name and the current timestamp. This ensures uniqueness across repeated
// runs even if the same project is re-extracted.
func generateJobID(packageName string) string {
	unixTimestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%d", packageName, unixTimestamp)
}

// movePackage atomically moves a package directory from source to destination.
//
// Uses os.Rename() for atomic same-filesystem moves. Falls back to a
// recursive copy + delete if rename fails (cross-device scenario).
func movePackage(sourcePath string, destinationPath string) error {
	// Ensure the destination's parent directory exists.
	destinationParent := filepath.Dir(destinationPath)
	if mkdirError := os.MkdirAll(destinationParent, 0755); mkdirError != nil {
		return fmt.Errorf("failed to create destination parent %s: %w", destinationParent, mkdirError)
	}

	// Attempt an atomic rename first — fastest path for same-filesystem moves.
	renameError := os.Rename(sourcePath, destinationPath)
	if renameError == nil {
		return nil
	}

	// Rename failed — likely a cross-device move. Fall back to copy + delete.
	if copyError := copyDirectory(sourcePath, destinationPath); copyError != nil {
		return fmt.Errorf("copy fallback failed for %s → %s: %w", sourcePath, destinationPath, copyError)
	}

	// Remove the source after successful copy.
	if removeError := os.RemoveAll(sourcePath); removeError != nil {
		// Copy succeeded but cleanup failed. The package exists in both locations.
		// This is safe but wasteful — log it and move on.
		return fmt.Errorf("copied successfully but failed to remove source %s: %w", sourcePath, removeError)
	}

	return nil
}

// moveToFailed moves a broken package to the failed directory for operator inspection.
// Errors during the move are silently ignored — failing to move a failed package
// should not crash the engine.
func moveToFailed(packagePath string, failedPath string) {
	packageName := filepath.Base(packagePath)
	failedDestination := filepath.Join(failedPath, packageName)

	// Best-effort move — silently ignore errors.
	_ = movePackage(packagePath, failedDestination)
}

// copyDirectory recursively copies a directory tree from source to destination.
// This is the fallback path when os.Rename fails due to cross-device moves.
func copyDirectory(sourcePath string, destinationPath string) error {
	// Create the destination root directory.
	if mkdirError := os.MkdirAll(destinationPath, 0755); mkdirError != nil {
		return fmt.Errorf("failed to create directory %s: %w", destinationPath, mkdirError)
	}

	// Walk the source directory tree and recreate it at the destination.
	entries, readError := os.ReadDir(sourcePath)
	if readError != nil {
		return fmt.Errorf("failed to read source directory %s: %w", sourcePath, readError)
	}

	for _, entry := range entries {
		sourceEntryPath := filepath.Join(sourcePath, entry.Name())
		destinationEntryPath := filepath.Join(destinationPath, entry.Name())

		if entry.IsDir() {
			// Recurse into subdirectories.
			if recursiveError := copyDirectory(sourceEntryPath, destinationEntryPath); recursiveError != nil {
				return recursiveError
			}
			continue
		}

		// Copy regular files using the existing copyFile helper from project.go.
		// That function handles open/create/io.Copy with proper error wrapping.
		if fileCopyError := copyFileForIntake(sourceEntryPath, destinationEntryPath); fileCopyError != nil {
			return fileCopyError
		}
	}

	return nil
}

// copyFileForIntake copies a single file from source to destination.
// Separate from project.go's copyFile to avoid coupling between packages.
func copyFileForIntake(sourcePath string, destinationPath string) error {
	sourceFile, openError := os.Open(sourcePath)
	if openError != nil {
		return fmt.Errorf("failed to open source %s: %w", sourcePath, openError)
	}
	defer sourceFile.Close()

	destinationFile, createError := os.Create(destinationPath)
	if createError != nil {
		return fmt.Errorf("failed to create destination %s: %w", destinationPath, createError)
	}
	defer destinationFile.Close()

	// Stream contents from source to destination.
	if _, copyError := io.Copy(destinationFile, sourceFile); copyError != nil {
		return fmt.Errorf("failed to copy %s → %s: %w", sourcePath, destinationPath, copyError)
	}

	return nil
}
