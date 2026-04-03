package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ChatProject represents a single project from ChatGPT Projects.
//
// This struct captures the minimal metadata needed to link ChatGPT
// projects to Bridgit's repository registry. Additional fields
// (description, created date, etc.) can be added as needed.
type ChatProject struct {
	// Name is the human-readable project name in ChatGPT.
	Name string

	// ID is the unique ChatGPT project identifier.
	// Used for fetching conversations and artifacts via the API.
	ID string
}

// chatProjectPayload mirrors the JSON structure written by extract_projects.py.
//
// The Python bridge writes a project.json file per project in its outbox.
// This struct only captures the fields we need for registry reconciliation.
type chatProjectPayload struct {
	// ProjectID is the ChatGPT project identifier (e.g., "g-p-...").
	ProjectID string `json:"project_id"`

	// DisplayName is the human-readable project name from ChatGPT.
	DisplayName string `json:"display_name"`
}

// FetchChatProjects reads ChatGPT project data from the bridge's outbox directory.
//
// The ChatGptToChatProjectsBridge writes one folder per project into its outbox,
// each containing a project.json file with the project ID and display name.
// This function scans that outbox directory to discover all available projects.
//
// The outbox path is relative to the repository root at:
//
//	bridges/ChatGptToChatProjectsBridge/outbox/
//
// If the outbox directory doesn't exist or is empty, this returns an empty slice
// (not an error), since the bridge may not have run yet.
func FetchChatProjects() ([]ChatProject, error) {
	// Resolve the outbox path relative to the working directory.
	// This assumes Bridgit is run from the repo root.
	outboxPath := filepath.Join("bridges", "ChatGptToChatProjectsBridge", "outbox")

	// Read all entries in the outbox directory.
	// Each subdirectory represents one ChatGPT project package.
	entries, err := os.ReadDir(outboxPath)
	if err != nil {
		// If the outbox doesn't exist, return empty — the bridge hasn't run yet.
		// This is expected behavior, not an error condition.
		if os.IsNotExist(err) {
			return []ChatProject{}, nil
		}
		// Any other error (permissions, I/O) is a real problem.
		return nil, fmt.Errorf("failed to read ChatProjects outbox at %s: %w", outboxPath, err)
	}

	var projects []ChatProject

	// Iterate through each subdirectory in the outbox.
	// Each folder should contain a project.json file written by the Python bridge.
	for _, entry := range entries {
		// Only process directories — files at the outbox root are ignored.
		if !entry.IsDir() {
			continue
		}

		// Build the path to the project.json file inside this package directory.
		projectFilePath := filepath.Join(outboxPath, entry.Name(), "project.json")

		// Attempt to read the project payload from disk.
		rawPayload, err := os.ReadFile(projectFilePath)
		if err != nil {
			// Skip directories that don't have a project.json.
			// They may be incomplete or in-progress packages.
			continue
		}

		// Parse the JSON payload into our internal struct.
		var payload chatProjectPayload
		if err := json.Unmarshal(rawPayload, &payload); err != nil {
			// Skip malformed JSON rather than failing the entire scan.
			// Log-worthy in production, but non-fatal for reconciliation.
			continue
		}

		// Use the display name from the payload, falling back to the folder name.
		// This ensures we always have a human-readable project name.
		projectName := payload.DisplayName
		if projectName == "" {
			projectName = entry.Name()
		}

		// Append the discovered project to our results.
		projects = append(projects, ChatProject{
			Name: projectName,
			ID:   payload.ProjectID,
		})
	}

	// Return all projects discovered in the outbox.
	return projects, nil
}
