// Package git — write.go provides git write operations for the provisioning phase.
//
// These functions mutate git state (clone, init, push) and are deliberately
// separated from git.go's read-only operations. Only called when the operator
// has explicitly opted into provisioning via EnableProvisioning config flag.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// IsGitRepo checks whether the given directory contains a .git subdirectory,
// indicating it has been initialized as a git repository. Returns false if
// the path does not exist or is not a directory.
func IsGitRepo(repoPath string) bool {
	// Check for the .git directory that git init creates.
	gitDir := repoPath + "/.git"
	fileInfo, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// HasOriginRemote checks whether the git repo at the given path has an
// "origin" remote configured. Returns false if the directory is not a git
// repo or if no origin remote exists.
func HasOriginRemote(repoPath string) bool {
	// Attempt to read the origin remote URL — failure means no remote.
	_, err := GetRemoteURL(repoPath)
	return err == nil
}

// CloneRepo runs `git clone <cloneURL> <destPath>` to create a local copy
// of a remote repository. Returns an error if the clone fails or the
// destination directory already exists.
func CloneRepo(cloneURL string, destPath string) error {
	// Verify the destination does not already exist to prevent data loss.
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("clone destination already exists: %s", destPath)
	}

	// Execute git clone targeting the specified destination path.
	cloneCommand := exec.Command("git", "clone", cloneURL, destPath)
	combinedOutput, err := cloneCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed for %s: %w (output: %s)",
			cloneURL, err, strings.TrimSpace(string(combinedOutput)))
	}
	return nil
}

// InitRepo runs `git init` in the given directory to initialize a new git
// repository. Returns an error if the directory does not exist or if git
// init fails for any reason.
func InitRepo(repoPath string) error {
	// Verify the directory exists before trying to initialize.
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("cannot init git repo — directory does not exist: %s", repoPath)
	}

	// Run git init inside the target directory.
	initCommand := exec.Command("git", "-C", repoPath, "init")
	combinedOutput, err := initCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git init failed in %s: %w (output: %s)",
			repoPath, err, strings.TrimSpace(string(combinedOutput)))
	}
	return nil
}

// AddRemote runs `git remote add <remoteName> <remoteURL>` to configure a
// new remote in the given repository. Returns an error if the remote already
// exists or the path is not a git repo.
func AddRemote(repoPath string, remoteName string, remoteURL string) error {
	// Add the remote using the standard git command.
	addCommand := exec.Command("git", "-C", repoPath, "remote", "add", remoteName, remoteURL)
	combinedOutput, err := addCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git remote add failed in %s: %w (output: %s)",
			repoPath, err, strings.TrimSpace(string(combinedOutput)))
	}
	return nil
}

// SetDefaultBranch runs `git branch -M main` to rename the current branch
// to "main". Called after git init to match GitHub's default branch naming
// convention, since older git versions default to "master".
func SetDefaultBranch(repoPath string) error {
	// Force-rename the current branch to "main" for GitHub compatibility.
	branchCommand := exec.Command("git", "-C", repoPath, "branch", "-M", "main")
	combinedOutput, err := branchCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch -M main failed in %s: %w (output: %s)",
			repoPath, err, strings.TrimSpace(string(combinedOutput)))
	}
	return nil
}

// CreateInitialCommit generates a default .gitignore (if none exists), stages
// all files, and creates an "Initial commit". If the repo is empty, a .gitkeep
// file is created to ensure there is something to commit.
func CreateInitialCommit(repoPath string) error {
	// Generate a default .gitignore before staging to prevent large files,
	// logs, and build artifacts from being committed to GitHub.
	ensureGitignore(repoPath)

	// Check if the directory has any files to commit.
	dirEntries, err := os.ReadDir(repoPath)
	if err != nil {
		return fmt.Errorf("cannot read directory for initial commit: %w", err)
	}

	// Count non-.git and non-.gitignore entries to determine if empty.
	fileCount := 0
	for _, dirEntry := range dirEntries {
		if dirEntry.Name() != ".git" {
			fileCount++
		}
	}

	// Create a .gitkeep placeholder if the directory has no real content.
	if fileCount == 0 {
		gitkeepPath := repoPath + "/.gitkeep"
		if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
			return fmt.Errorf("failed to create .gitkeep in %s: %w", repoPath, err)
		}
	}

	// Stage all files in the repository for the initial commit.
	addCommand := exec.Command("git", "-C", repoPath, "add", ".")
	combinedOutput, err := addCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed in %s: %w (output: %s)",
			repoPath, err, strings.TrimSpace(string(combinedOutput)))
	}

	// Create the initial commit with a standard message.
	commitCommand := exec.Command("git", "-C", repoPath, "commit", "-m", "Initial commit")
	combinedOutput, err = commitCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git commit failed in %s: %w (output: %s)",
			repoPath, err, strings.TrimSpace(string(combinedOutput)))
	}
	return nil
}

// defaultGitignore contains patterns for files that should never be committed
// to GitHub. Covers logs, build artifacts, binaries, environment files, and
// common runtime directories that tend to contain large files.
const defaultGitignore = `# Logs
logs/
*.log

# Build artifacts and binaries
*.exe
*.dll
*.so
*.dylib
*.out
bin/
dist/
build/

# Runtime and temp files
*.pid
*.swp
*.swo
tmp/

# Environment and secrets
.env
.env.*
*.pem
*.key

# OS files
.DS_Store
Thumbs.db

# IDE
.idea/
.vscode/
*.code-workspace

# Dependencies (language-specific)
node_modules/
vendor/
__pycache__/
*.pyc
`

// ensureGitignore creates a default .gitignore in the repo directory if one
// does not already exist. If a .gitignore is present, it is left untouched
// to respect any existing project-specific ignore rules.
func ensureGitignore(repoPath string) {
	gitignorePath := repoPath + "/.gitignore"

	// Do not overwrite an existing .gitignore — the project may already
	// have custom rules that we should not disturb.
	if _, err := os.Stat(gitignorePath); err == nil {
		return
	}

	// Write the default .gitignore. Errors are silently ignored because
	// a missing .gitignore is non-fatal — the commit will still succeed,
	// it just might include files that should have been excluded.
	_ = os.WriteFile(gitignorePath, []byte(defaultGitignore), 0644)
}

// PushToRemote runs `git push -u <remoteName> <branch>` to push the local
// branch to the remote and set up tracking. Returns an error if the push
// fails (e.g., authentication issues, network problems).
func PushToRemote(repoPath string, remoteName string, branch string) error {
	// Push with upstream tracking so future pulls work without arguments.
	pushCommand := exec.Command("git", "-C", repoPath, "push", "-u", remoteName, branch)
	combinedOutput, err := pushCommand.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git push failed in %s: %w (output: %s)",
			repoPath, err, strings.TrimSpace(string(combinedOutput)))
	}
	return nil
}
