package sync

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/go-github/v62/github"
)

// GitHubRepo represents a single repository from GitHub.
//
// This struct captures the essential metadata for linking GitHub repos
// to Bridgit's registry. Additional fields (description, stars, language)
// can be added as the use case expands.
type GitHubRepo struct {
	// Name is the repository name (e.g., "BridgitBridgerton").
	Name string

	// URL is the full clone URL for git operations.
	// Example: "https://github.com/CraigThomasParsons/BridgitBridgerton.git"
	URL string
}

// FetchGitHubRepos retrieves all public repositories for the given GitHub owner.
//
// Uses the GitHub REST API v3 via go-github. If the GITHUB_TOKEN environment
// variable is set, it authenticates for higher rate limits and access to private
// repos. Without a token, only public repos are returned (60 req/hr limit).
//
// Handles pagination automatically — GitHub returns 100 repos per page max.
// All pages are fetched sequentially until no more results remain.
func FetchGitHubRepos(owner string) ([]GitHubRepo, error) {
	// Create a context with timeout to prevent hanging on slow API responses.
	// 30 seconds is generous — most GitHub API calls complete in <2 seconds.
	requestContext, cancelRequest := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelRequest()

	// Build the GitHub client. If GITHUB_TOKEN is set, use it for authentication.
	// Authenticated requests get 5000 req/hr vs 60 req/hr for anonymous.
	var githubClient *github.Client
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken != "" {
		// Authenticated client — higher rate limits and private repo access.
		githubClient = github.NewClient(nil).WithAuthToken(githubToken)
	} else {
		// Anonymous client — only public repos, lower rate limit.
		githubClient = github.NewClient(nil)
	}

	// Configure pagination options to fetch 100 repos per page (maximum).
	// This minimizes the number of API calls for users with many repos.
	listOptions := &github.RepositoryListByUserOptions{
		Type: "owner",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allRepos []GitHubRepo

	// Paginate through all pages of results.
	// GitHub signals the last page by returning an empty NextPage (0).
	for {
		// Fetch one page of repos from the GitHub API.
		pageOfRepos, apiResponse, err := githubClient.Repositories.ListByUser(
			requestContext, owner, listOptions,
		)
		if err != nil {
			// Wrap the error with context for operator triage.
			// Include the owner name so operators know which account failed.
			return nil, fmt.Errorf("failed to fetch GitHub repos for %s: %w", owner, err)
		}

		// Convert each GitHub repo into our internal GitHubRepo struct.
		// We only extract name and clone URL — additional fields can be added later.
		for _, repo := range pageOfRepos {
			allRepos = append(allRepos, GitHubRepo{
				Name: repo.GetName(),
				URL:  repo.GetCloneURL(),
			})
		}

		// Check if there are more pages to fetch.
		// NextPage is 0 when we've reached the last page.
		if apiResponse.NextPage == 0 {
			break
		}

		// Advance to the next page for the next iteration.
		listOptions.Page = apiResponse.NextPage
	}

	// Return all discovered repos across all pages.
	return allRepos, nil
}
