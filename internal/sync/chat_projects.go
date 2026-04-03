package sync

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

// FetchChatProjects retrieves all ChatGPT projects for the current user.
//
// This is currently a stub implementation that returns an empty list.
// The real implementation will connect to the ChatGptToChatProjectsBridge
// to read from its outbox directory or call its token-authenticated API.
//
// TODO: Connect to ChatGptToChatProjectsBridge inbox/outbox pipeline.
// TODO: Add authentication token handling via the bridge's token API.
func FetchChatProjects() ([]ChatProject, error) {
	// Stub: return empty list until bridge integration is complete.
	// This allows the engine to compile and run without blocking on bridge work.
	return []ChatProject{}, nil
}
