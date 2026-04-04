// Package sync — provision.go implements automated repo provisioning for Phase 5b.
//
// After reconciliation detects misalignment (MISSING_LOCAL, MISSING_GITHUB, etc.),
// the provisioning phase acts on those findings by creating directories, cloning
// repos, initializing git, and creating GitHub repos. States that require human
// judgment are flagged for review rather than acted on automatically.
package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"bridgit/internal/config"
	"bridgit/internal/contracts"
	"bridgit/internal/git"
	"bridgit/internal/registry"
)

// ProvisionResult describes the outcome of a single provisioning action.
// Each registry entry that is evaluated by the provisioning phase produces
// one result, whether it was acted on, skipped, or flagged.
type ProvisionResult struct {
	// RepoID is the registry ID of the repo being provisioned.
	RepoID string

	// Action describes what happened: CREATED_BOTH, CLONED, INITIALIZED,
	// LINKED, FLAGGED_REVIEW, SKIPPED, or FAILED.
	Action string

	// Details contains human-readable explanation lines for the operator.
	Details []string
}

// ProvisionRepos inspects all registry entries and provisions missing repos
// based on the decision matrix. Returns a slice of results describing each
// action taken. Only entries that need provisioning are included in results;
// already-aligned entries are silently skipped.
func ProvisionRepos(
	registryData *registry.Registry,
	cfg *config.Config,
	githubRepos []GitHubRepo,
	emitter contracts.Emitter,
) []ProvisionResult {
	// Build a set of GitHub repo names for fast existence checks.
	// Case-insensitive to match how the reconciler operates.
	githubNameSet := buildGitHubNameSet(githubRepos)

	// Build a lookup from lowercase GitHub name to clone URL.
	githubURLMap := buildGitHubURLMap(githubRepos)

	var allResults []ProvisionResult

	// Evaluate each registry entry against the decision matrix.
	for repoIndex := range registryData.Repos {
		// Use a pointer so provisioning actions can mutate the entry in-place.
		registryRepo := &registryData.Repos[repoIndex]

		// Classify the current state of this entry.
		classification := classifyRepoState(registryRepo, githubNameSet, cfg)

		// Skip entries that are already aligned or have incomplete data.
		if classification == "aligned" || classification == "incomplete_entry" {
			continue
		}

		// Execute the provisioning action for this classification.
		provisionResult := provisionSingleRepo(
			registryRepo, classification, cfg, githubURLMap, emitter,
		)
		allResults = append(allResults, provisionResult)
	}

	return allResults
}

// classifyRepoState inspects a single registry entry and returns a string
// describing its local-vs-GitHub alignment. This classification drives
// which provisioning action (if any) is taken.
func classifyRepoState(
	registryRepo *registry.Repo,
	githubNameSet map[string]bool,
	cfg *config.Config,
) string {
	// Determine the expected local path — either from registry or inferred.
	localPath := registryRepo.Local.Path
	if localPath == "" && registryRepo.GitHub.Name != "" {
		// Infer local path from GitHub name if not explicitly set.
		localPath = filepath.Join(cfg.CodeRoot, registryRepo.GitHub.Name)
	}

	// Check whether this entry has enough information to act on.
	if localPath == "" && registryRepo.GitHub.Name == "" {
		return "incomplete_entry"
	}

	// Determine local filesystem state.
	localFolderExists := false
	localIsGitRepo := false
	localHasOriginRemote := false

	if localPath != "" {
		if _, statError := os.Stat(localPath); statError == nil {
			localFolderExists = true
			localIsGitRepo = git.IsGitRepo(localPath)
			if localIsGitRepo {
				localHasOriginRemote = git.HasOriginRemote(localPath)
			}
		}
	}

	// Determine GitHub state using the pre-built name set.
	githubRepoExists := false
	if registryRepo.GitHub.Name != "" {
		githubRepoExists = githubNameSet[strings.ToLower(registryRepo.GitHub.Name)]
	}

	// Apply the decision matrix to classify this entry.
	// Each branch matches one row in the provisioning decision table.
	if localFolderExists && localIsGitRepo && localHasOriginRemote {
		return "aligned"
	}
	if !localFolderExists && !githubRepoExists {
		return "no_folder_no_repo"
	}
	if !localFolderExists && githubRepoExists {
		return "no_folder_repo_exists"
	}
	if localFolderExists && !localIsGitRepo && githubRepoExists {
		return "folder_nogit_repo_exists"
	}
	if localFolderExists && !localIsGitRepo && !githubRepoExists {
		return "folder_nogit_no_repo"
	}
	if localFolderExists && localIsGitRepo && !localHasOriginRemote && githubRepoExists {
		return "folder_git_noremote_repo_exists"
	}
	if localFolderExists && localIsGitRepo && !localHasOriginRemote && !githubRepoExists {
		return "folder_git_noremote_no_repo"
	}

	// Fallback — should not be reached if all states are covered above.
	return "aligned"
}

// provisionSingleRepo handles provisioning for one registry entry based on
// its classified state. Returns a ProvisionResult describing what happened.
func provisionSingleRepo(
	registryRepo *registry.Repo,
	classification string,
	cfg *config.Config,
	githubURLMap map[string]string,
	emitter contracts.Emitter,
) ProvisionResult {
	switch classification {
	case "no_folder_no_repo":
		return provisionCreateBoth(registryRepo, cfg, emitter)

	case "no_folder_repo_exists":
		return provisionClone(registryRepo, cfg, githubURLMap, emitter)

	case "folder_nogit_repo_exists":
		return provisionFolderWithExistingRepo(registryRepo, cfg, githubURLMap, emitter)

	case "folder_nogit_no_repo":
		return provisionInitAndCreateRepo(registryRepo, cfg, emitter)

	case "folder_git_noremote_repo_exists":
		return provisionLinkExistingRepo(registryRepo, cfg, githubURLMap, emitter)

	case "folder_git_noremote_no_repo":
		return provisionCreateRepoAndPush(registryRepo, cfg, emitter)

	default:
		return ProvisionResult{
			RepoID:  registryRepo.ID,
			Action:  "SKIPPED",
			Details: []string{fmt.Sprintf("unhandled classification: %s", classification)},
		}
	}
}

// provisionCreateBoth creates a GitHub repo and local folder from scratch.
// Used when a registry entry has no local path and no GitHub repo.
func provisionCreateBoth(
	registryRepo *registry.Repo,
	cfg *config.Config,
	emitter contracts.Emitter,
) ProvisionResult {
	repoName := registryRepo.ID
	localPath := filepath.Join(cfg.CodeRoot, repoName)

	// Create the local directory to hold the new repo.
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return failedResult(registryRepo.ID, "create directory", err)
	}

	// Create the GitHub repo via the gh CLI.
	cloneURL, err := createGitHubRepo(cfg.GitHubOwner, repoName)
	if err != nil {
		return failedResult(registryRepo.ID, "create GitHub repo", err)
	}

	// Initialize git, set branch, add remote, commit, and push.
	if err := initAndPush(localPath, cloneURL); err != nil {
		return failedResult(registryRepo.ID, "init and push", err)
	}

	// Update the registry entry with the new local and GitHub metadata.
	registryRepo.Local.Path = localPath
	registryRepo.GitHub.Name = repoName
	registryRepo.GitHub.URL = cloneURL

	// Emit a success event for downstream subscribers.
	emitter.Emit(contracts.Event{
		Type:    contracts.RepoProvisioned,
		Phase:   "provision",
		RepoID:  registryRepo.ID,
		Message: fmt.Sprintf("created GitHub repo and local folder for %s", repoName),
		Metadata: map[string]string{
			"action":     "CREATED_BOTH",
			"local_path": localPath,
			"clone_url":  cloneURL,
		},
	})

	return ProvisionResult{
		RepoID: registryRepo.ID,
		Action: "CREATED_BOTH",
		Details: []string{
			fmt.Sprintf("created GitHub repo %s/%s", cfg.GitHubOwner, repoName),
			fmt.Sprintf("initialized local repo at %s", localPath),
			"pushed initial commit to origin/main",
		},
	}
}

// provisionClone clones an existing GitHub repo into the local Code folder.
// Used when the GitHub repo exists but no local folder does.
func provisionClone(
	registryRepo *registry.Repo,
	cfg *config.Config,
	githubURLMap map[string]string,
	emitter contracts.Emitter,
) ProvisionResult {
	// Determine the clone URL from the registry or the GitHub API data.
	cloneURL := registryRepo.GitHub.URL
	if cloneURL == "" {
		cloneURL = githubURLMap[strings.ToLower(registryRepo.GitHub.Name)]
	}
	if cloneURL == "" {
		cloneURL = buildCloneURL(cfg.GitHubOwner, registryRepo.GitHub.Name)
	}

	// Determine the destination path in the Code folder.
	destPath := filepath.Join(cfg.CodeRoot, registryRepo.GitHub.Name)

	// Execute the clone operation.
	if err := git.CloneRepo(cloneURL, destPath); err != nil {
		return failedResult(registryRepo.ID, "clone", err)
	}

	// Update the registry entry with the local path.
	registryRepo.Local.Path = destPath
	if registryRepo.GitHub.URL == "" {
		registryRepo.GitHub.URL = cloneURL
	}

	// Emit a success event for the clone action.
	emitter.Emit(contracts.Event{
		Type:    contracts.RepoProvisioned,
		Phase:   "provision",
		RepoID:  registryRepo.ID,
		Message: fmt.Sprintf("cloned %s into %s", registryRepo.GitHub.Name, destPath),
		Metadata: map[string]string{
			"action":     "CLONED",
			"local_path": destPath,
			"clone_url":  cloneURL,
		},
	})

	return ProvisionResult{
		RepoID: registryRepo.ID,
		Action: "CLONED",
		Details: []string{
			fmt.Sprintf("cloned %s", cloneURL),
			fmt.Sprintf("local path: %s", destPath),
		},
	}
}

// provisionFolderWithExistingRepo handles the case where a local folder exists
// without git, and a GitHub repo also exists. If the GitHub repo is empty,
// it initializes and pushes. If not, it flags for review.
func provisionFolderWithExistingRepo(
	registryRepo *registry.Repo,
	cfg *config.Config,
	githubURLMap map[string]string,
	emitter contracts.Emitter,
) ProvisionResult {
	// Check whether the GitHub repo has any content.
	repoEmpty, err := isGitHubRepoEmpty(cfg.GitHubOwner, registryRepo.GitHub.Name)
	if err != nil {
		return failedResult(registryRepo.ID, "check if GitHub repo is empty", err)
	}

	localPath := registryRepo.Local.Path
	if localPath == "" {
		localPath = filepath.Join(cfg.CodeRoot, registryRepo.GitHub.Name)
	}

	// If GitHub repo has content, we cannot safely auto-link — flag for review.
	if !repoEmpty {
		emitter.Emit(contracts.Event{
			Type:    contracts.ProvisioningReviewNeeded,
			Phase:   "provision",
			RepoID:  registryRepo.ID,
			Message: fmt.Sprintf("local folder %s has code but GitHub repo %s also has commits", localPath, registryRepo.GitHub.Name),
		})
		return ProvisionResult{
			RepoID: registryRepo.ID,
			Action: "FLAGGED_REVIEW",
			Details: []string{
				fmt.Sprintf("local folder %s has untracked code", localPath),
				fmt.Sprintf("GitHub repo %s/%s has existing commits", cfg.GitHubOwner, registryRepo.GitHub.Name),
				"manual review required before linking",
			},
		}
	}

	// GitHub repo is empty — safe to initialize and push local code.
	cloneURL := registryRepo.GitHub.URL
	if cloneURL == "" {
		cloneURL = githubURLMap[strings.ToLower(registryRepo.GitHub.Name)]
	}
	if cloneURL == "" {
		cloneURL = buildCloneURL(cfg.GitHubOwner, registryRepo.GitHub.Name)
	}

	// Initialize git, set branch, add remote, commit, and push.
	if err := initAndPush(localPath, cloneURL); err != nil {
		return failedResult(registryRepo.ID, "init and push", err)
	}

	// Update registry with the confirmed local path and clone URL.
	registryRepo.Local.Path = localPath
	if registryRepo.GitHub.URL == "" {
		registryRepo.GitHub.URL = cloneURL
	}

	emitter.Emit(contracts.Event{
		Type:    contracts.RepoProvisioned,
		Phase:   "provision",
		RepoID:  registryRepo.ID,
		Message: fmt.Sprintf("initialized and pushed %s to empty GitHub repo", localPath),
		Metadata: map[string]string{
			"action":     "INITIALIZED",
			"local_path": localPath,
		},
	})

	return ProvisionResult{
		RepoID: registryRepo.ID,
		Action: "INITIALIZED",
		Details: []string{
			fmt.Sprintf("initialized git in %s", localPath),
			fmt.Sprintf("pushed to empty GitHub repo %s/%s", cfg.GitHubOwner, registryRepo.GitHub.Name),
		},
	}
}

// provisionInitAndCreateRepo initializes git and creates a new GitHub repo
// for a local folder that has no git and no GitHub presence.
func provisionInitAndCreateRepo(
	registryRepo *registry.Repo,
	cfg *config.Config,
	emitter contracts.Emitter,
) ProvisionResult {
	localPath := registryRepo.Local.Path
	repoName := registryRepo.ID

	// Create the GitHub repo first so we have the remote URL.
	cloneURL, err := createGitHubRepo(cfg.GitHubOwner, repoName)
	if err != nil {
		return failedResult(registryRepo.ID, "create GitHub repo", err)
	}

	// Initialize git, set branch, add remote, commit, and push.
	if err := initAndPush(localPath, cloneURL); err != nil {
		return failedResult(registryRepo.ID, "init and push", err)
	}

	// Update registry with GitHub metadata.
	registryRepo.GitHub.Name = repoName
	registryRepo.GitHub.URL = cloneURL

	emitter.Emit(contracts.Event{
		Type:    contracts.RepoProvisioned,
		Phase:   "provision",
		RepoID:  registryRepo.ID,
		Message: fmt.Sprintf("initialized %s and created GitHub repo %s", localPath, repoName),
		Metadata: map[string]string{
			"action":     "CREATED_BOTH",
			"local_path": localPath,
			"clone_url":  cloneURL,
		},
	})

	return ProvisionResult{
		RepoID: registryRepo.ID,
		Action: "CREATED_BOTH",
		Details: []string{
			fmt.Sprintf("created GitHub repo %s/%s", cfg.GitHubOwner, repoName),
			fmt.Sprintf("initialized git in existing folder %s", localPath),
			"pushed to origin/main",
		},
	}
}

// provisionLinkExistingRepo adds an origin remote and pushes to an existing
// GitHub repo from a local git repo that has no remote configured.
func provisionLinkExistingRepo(
	registryRepo *registry.Repo,
	cfg *config.Config,
	githubURLMap map[string]string,
	emitter contracts.Emitter,
) ProvisionResult {
	// Check whether the GitHub repo has content — if so, flag for review.
	repoEmpty, err := isGitHubRepoEmpty(cfg.GitHubOwner, registryRepo.GitHub.Name)
	if err != nil {
		return failedResult(registryRepo.ID, "check if GitHub repo is empty", err)
	}

	localPath := registryRepo.Local.Path

	// If GitHub repo has commits, flag for manual review to avoid conflicts.
	if !repoEmpty {
		emitter.Emit(contracts.Event{
			Type:    contracts.ProvisioningReviewNeeded,
			Phase:   "provision",
			RepoID:  registryRepo.ID,
			Message: fmt.Sprintf("local git repo %s has no remote, GitHub repo %s has commits", localPath, registryRepo.GitHub.Name),
		})
		return ProvisionResult{
			RepoID: registryRepo.ID,
			Action: "FLAGGED_REVIEW",
			Details: []string{
				fmt.Sprintf("local git repo %s has no origin remote", localPath),
				fmt.Sprintf("GitHub repo %s/%s has existing commits", cfg.GitHubOwner, registryRepo.GitHub.Name),
				"manual review required — potential code divergence",
			},
		}
	}

	// GitHub repo is empty — safe to add remote and push.
	cloneURL := registryRepo.GitHub.URL
	if cloneURL == "" {
		cloneURL = githubURLMap[strings.ToLower(registryRepo.GitHub.Name)]
	}
	if cloneURL == "" {
		cloneURL = buildCloneURL(cfg.GitHubOwner, registryRepo.GitHub.Name)
	}

	// Add origin remote and push the existing local commits.
	if err := git.AddRemote(localPath, "origin", cloneURL); err != nil {
		return failedResult(registryRepo.ID, "add remote", err)
	}
	if err := git.PushToRemote(localPath, "origin", "main"); err != nil {
		return failedResult(registryRepo.ID, "push to remote", err)
	}

	// Update registry with clone URL if it was missing.
	if registryRepo.GitHub.URL == "" {
		registryRepo.GitHub.URL = cloneURL
	}

	emitter.Emit(contracts.Event{
		Type:    contracts.RepoProvisioned,
		Phase:   "provision",
		RepoID:  registryRepo.ID,
		Message: fmt.Sprintf("linked %s to GitHub repo %s", localPath, registryRepo.GitHub.Name),
		Metadata: map[string]string{
			"action":     "LINKED",
			"local_path": localPath,
			"clone_url":  cloneURL,
		},
	})

	return ProvisionResult{
		RepoID: registryRepo.ID,
		Action: "LINKED",
		Details: []string{
			fmt.Sprintf("added origin remote to %s", localPath),
			fmt.Sprintf("pushed to %s/%s", cfg.GitHubOwner, registryRepo.GitHub.Name),
		},
	}
}

// provisionCreateRepoAndPush creates a GitHub repo and pushes an existing
// local git repo (that has no remote) to it.
func provisionCreateRepoAndPush(
	registryRepo *registry.Repo,
	cfg *config.Config,
	emitter contracts.Emitter,
) ProvisionResult {
	localPath := registryRepo.Local.Path
	repoName := registryRepo.ID

	// Create the GitHub repo to get a remote URL.
	cloneURL, err := createGitHubRepo(cfg.GitHubOwner, repoName)
	if err != nil {
		return failedResult(registryRepo.ID, "create GitHub repo", err)
	}

	// Add the remote and push existing commits.
	if err := git.AddRemote(localPath, "origin", cloneURL); err != nil {
		return failedResult(registryRepo.ID, "add remote", err)
	}
	if err := git.PushToRemote(localPath, "origin", "main"); err != nil {
		return failedResult(registryRepo.ID, "push to remote", err)
	}

	// Update registry with GitHub metadata.
	registryRepo.GitHub.Name = repoName
	registryRepo.GitHub.URL = cloneURL

	emitter.Emit(contracts.Event{
		Type:    contracts.RepoProvisioned,
		Phase:   "provision",
		RepoID:  registryRepo.ID,
		Message: fmt.Sprintf("created GitHub repo %s and pushed from %s", repoName, localPath),
		Metadata: map[string]string{
			"action":     "LINKED",
			"local_path": localPath,
			"clone_url":  cloneURL,
		},
	})

	return ProvisionResult{
		RepoID: registryRepo.ID,
		Action: "LINKED",
		Details: []string{
			fmt.Sprintf("created GitHub repo %s/%s", cfg.GitHubOwner, repoName),
			fmt.Sprintf("pushed existing commits from %s", localPath),
		},
	}
}

// --- Helper Functions ---

// initAndPush runs the full sequence: git init, set branch to main, add
// origin remote, create initial commit (if needed), and push. This is the
// common path for CREATED_BOTH and INITIALIZED actions.
func initAndPush(localPath string, cloneURL string) error {
	// Initialize a new git repository in the directory.
	if err := git.InitRepo(localPath); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}

	// Rename the default branch to "main" for GitHub compatibility.
	if err := git.SetDefaultBranch(localPath); err != nil {
		return fmt.Errorf("set default branch failed: %w", err)
	}

	// Add the GitHub repo as the origin remote.
	if err := git.AddRemote(localPath, "origin", cloneURL); err != nil {
		return fmt.Errorf("add remote failed: %w", err)
	}

	// Stage and commit all files (creates .gitkeep if directory is empty).
	if err := git.CreateInitialCommit(localPath); err != nil {
		return fmt.Errorf("initial commit failed: %w", err)
	}

	// Push the initial commit to GitHub.
	if err := git.PushToRemote(localPath, "origin", "main"); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	return nil
}

// createGitHubRepo creates a new GitHub repository using the gh CLI.
// Returns the HTTPS clone URL on success. Creates a public repo by default.
func createGitHubRepo(owner string, repoName string) (string, error) {
	// Use the gh CLI which handles authentication via the system keyring.
	// The --public flag creates an open-source repo (user's preference).
	fullRepoName := fmt.Sprintf("%s/%s", owner, repoName)
	ghCommand := exec.Command("gh", "repo", "create", fullRepoName, "--public")

	combinedOutput, err := ghCommand.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh repo create failed for %s: %w (output: %s)",
			fullRepoName, err, strings.TrimSpace(string(combinedOutput)))
	}

	// Construct the clone URL from the known owner and repo name.
	cloneURL := buildCloneURL(owner, repoName)
	return cloneURL, nil
}

// isGitHubRepoEmpty checks whether a GitHub repository has any commits by
// querying the GitHub API via the gh CLI. Returns true if the repo has
// zero commits (i.e., was just created and never pushed to).
func isGitHubRepoEmpty(owner string, repoName string) (bool, error) {
	// Use gh api to check the repository's size field.
	// A repo with size 0 has no content (no commits, no files).
	fullRepoName := fmt.Sprintf("%s/%s", owner, repoName)
	ghCommand := exec.Command("gh", "api", fmt.Sprintf("repos/%s", fullRepoName),
		"--jq", ".size")

	rawOutput, err := ghCommand.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check if %s is empty: %w", fullRepoName, err)
	}

	// Parse the size — "0" means the repo is empty.
	sizeString := strings.TrimSpace(string(rawOutput))
	return sizeString == "0", nil
}

// buildCloneURL constructs a GitHub HTTPS clone URL from owner and repo name.
func buildCloneURL(owner string, repoName string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repoName)
}

// buildGitHubURLMap creates a lookup from lowercase GitHub repo name to its
// clone URL. Used to resolve clone URLs for repos discovered via the API.
func buildGitHubURLMap(githubRepos []GitHubRepo) map[string]string {
	urlMap := make(map[string]string, len(githubRepos))
	for _, githubRepo := range githubRepos {
		urlMap[strings.ToLower(githubRepo.Name)] = githubRepo.URL
	}
	return urlMap
}

// failedResult creates a ProvisionResult for a failed provisioning action.
// Centralizes the FAILED result pattern to reduce duplication.
func failedResult(repoID string, operation string, err error) ProvisionResult {
	return ProvisionResult{
		RepoID:  repoID,
		Action:  "FAILED",
		Details: []string{fmt.Sprintf("failed to %s: %v", operation, err)},
	}
}
