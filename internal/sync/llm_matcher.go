// Package sync — llm_matcher.go implements LLM-assisted fuzzy matching
// for ChatProjects enrichment.
//
// When strict name matching fails to link ChatProjects names to registry
// entries (e.g., "The Postal Service" vs "thepostalservice"), this module
// batches the unmatched names and asks an LLM to resolve the pairings.
// Uses the OpenAI-compatible chat completions API (works with Groq, OpenAI,
// and other compatible providers).

package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// LLMMatchResult captures one ChatProjects-to-registry pairing
// as determined by the LLM. Each result maps a human-readable
// project name to a slug-style registry ID.
type LLMMatchResult struct {
	// ChatProjectName is the display name from ChatProjects (e.g., "The Postal Service").
	ChatProjectName string `json:"chat_name"`

	// RepoID is the registry ID the LLM matched it to, or empty if no match.
	RepoID string `json:"repo_id"`

	// Confidence indicates how certain the LLM is: "high", "medium", or "low".
	Confidence string `json:"confidence"`
}

// chatCompletionRequest is the payload structure for the OpenAI-compatible
// chat completions API. Works with Groq, OpenAI, and similar providers.
type chatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []chatCompletionMessage `json:"messages"`
}

// chatCompletionMessage represents a single conversation turn in the API request.
type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the top-level response from the chat completions API.
type chatCompletionResponse struct {
	Choices []chatCompletionChoice `json:"choices"`
}

// chatCompletionChoice represents one completion choice in the API response.
// The LLM's text output lives in Message.Content.
type chatCompletionChoice struct {
	Message chatCompletionMessage `json:"message"`
}

// ResolveFuzzyMatches sends unmatched ChatProjects names and registry IDs
// to an OpenAI-compatible LLM API for intelligent fuzzy matching.
//
// This function is only called when strict matching leaves items unresolved.
// It batches all unmatched items into a single API call to minimize cost.
// Returns an empty slice (not an error) if the API call fails — fuzzy
// matching is best-effort and should never block the sync pipeline.
func ResolveFuzzyMatches(
	unmatchedChatProjects []ChatProject,
	registryIDs []string,
	apiKey string,
	baseURL string,
	model string,
) ([]LLMMatchResult, error) {

	// Build the prompt with both lists for the LLM to cross-reference.
	matchingPrompt := buildFuzzyMatchPrompt(unmatchedChatProjects, registryIDs)

	// Construct the chat completions request payload.
	requestPayload := chatCompletionRequest{
		Model: model,
		Messages: []chatCompletionMessage{
			{Role: "user", Content: matchingPrompt},
		},
	}

	// Marshal the request to JSON for the HTTP body.
	requestBytes, marshalError := json.Marshal(requestPayload)
	if marshalError != nil {
		return nil, fmt.Errorf("failed to marshal LLM request: %w", marshalError)
	}

	// Send the request to the chat completions endpoint.
	apiURL := strings.TrimRight(baseURL, "/") + "/chat/completions"
	httpRequest, requestError := http.NewRequest("POST", apiURL, bytes.NewReader(requestBytes))
	if requestError != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", requestError)
	}

	// Set the standard OpenAI-compatible headers for authentication.
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)

	// Use a generous timeout — network and inference can be slow.
	httpClient := &http.Client{Timeout: 30 * time.Second}
	httpResponse, httpError := httpClient.Do(httpRequest)
	if httpError != nil {
		return nil, fmt.Errorf("LLM API request failed: %w", httpError)
	}
	defer httpResponse.Body.Close()

	// Read the full response body for parsing.
	responseBytes, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		return nil, fmt.Errorf("failed to read LLM response body: %w", readError)
	}

	// Check for non-200 status codes from the API.
	if httpResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API returned status %d: %s", httpResponse.StatusCode, string(responseBytes))
	}

	// Parse the chat completions response to extract the message content.
	var apiResponse chatCompletionResponse
	if unmarshalError := json.Unmarshal(responseBytes, &apiResponse); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", unmarshalError)
	}

	// Extract the text content from the first choice's message.
	if len(apiResponse.Choices) == 0 {
		return nil, fmt.Errorf("LLM response contains no choices")
	}

	// Parse the LLM's JSON output into structured match results.
	rawLLMText := apiResponse.Choices[0].Message.Content
	matchResults, parseError := parseLLMMatchResponse(rawLLMText)
	if parseError != nil {
		return nil, fmt.Errorf("failed to parse LLM match output: %w", parseError)
	}

	return matchResults, nil
}

// buildFuzzyMatchPrompt constructs the prompt that asks the LLM to pair
// unmatched ChatProjects names with registry IDs. The prompt is designed
// to produce clean JSON output with no preamble or explanation.
func buildFuzzyMatchPrompt(unmatchedProjects []ChatProject, registryIDs []string) string {
	var promptBuilder strings.Builder

	// System-level instruction for the matching task.
	promptBuilder.WriteString("You are matching ChatGPT project names to code repository IDs.\n")
	promptBuilder.WriteString("Project names use human-readable titles with spaces and capitals.\n")
	promptBuilder.WriteString("Repository IDs are lowercase slugs (sometimes with hyphens removed).\n\n")

	// List the unmatched project names that need resolution.
	promptBuilder.WriteString("Unmatched project names (from ChatGPT):\n")
	for _, chatProject := range unmatchedProjects {
		promptBuilder.WriteString(fmt.Sprintf("- %s\n", chatProject.Name))
	}

	// List all registry IDs as the candidate pool for matching.
	promptBuilder.WriteString("\nKnown repository IDs:\n")
	for _, registryID := range registryIDs {
		promptBuilder.WriteString(fmt.Sprintf("- %s\n", registryID))
	}

	// Instruction for the output format — strict JSON, no markdown fences.
	promptBuilder.WriteString("\nFor each project name, find the most likely matching repo ID.\n")
	promptBuilder.WriteString("Consider that project names may have spaces, articles (The, A), or\n")
	promptBuilder.WriteString("different word forms that map to concatenated or hyphenated IDs.\n")
	promptBuilder.WriteString("Examples: 'The Postal Service' -> 'thepostalservice', 'Claudia code' -> 'claudiacode'\n\n")
	promptBuilder.WriteString("Return ONLY a JSON array, no markdown fences, no explanation:\n")
	promptBuilder.WriteString(`[{"chat_name": "...", "repo_id": "...", "confidence": "high|medium|low"}]` + "\n")
	promptBuilder.WriteString("If no match exists for a project, set repo_id to empty string.\n")

	return promptBuilder.String()
}

// parseLLMMatchResponse extracts structured match results from the LLM's
// text output. Handles both clean JSON and JSON wrapped in markdown fences.
func parseLLMMatchResponse(rawText string) ([]LLMMatchResult, error) {
	// Strip markdown code fences if the LLM wrapped its output.
	cleanedText := strings.TrimSpace(rawText)
	cleanedText = strings.TrimPrefix(cleanedText, "```json")
	cleanedText = strings.TrimPrefix(cleanedText, "```")
	cleanedText = strings.TrimSuffix(cleanedText, "```")
	cleanedText = strings.TrimSpace(cleanedText)

	// Parse the JSON array into match results.
	var matchResults []LLMMatchResult
	if unmarshalError := json.Unmarshal([]byte(cleanedText), &matchResults); unmarshalError != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (raw: %s)", unmarshalError, cleanedText[:min(len(cleanedText), 200)])
	}

	return matchResults, nil
}

// min returns the smaller of two integers. Used for safe string truncation
// in error messages when the LLM output might be very long.
func min(valueA int, valueB int) int {
	if valueA < valueB {
		return valueA
	}
	return valueB
}
