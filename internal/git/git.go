// Package git provides git operations for Bridgit repository management.
//
// This package will contain functions for cloning repos, checking remote status,
// validating git directories, and performing automated commits/pushes as part
// of the sync pipeline. Currently unused but reserved for future git automation.
//
// TODO: Add ValidateGitRepo(path string) to check if a directory is a git repo.
// TODO: Add FetchRemoteStatus(path string) to compare local/remote branches.
// TODO: Add CloneRepo(url, destination string) for automated repo setup.
package git
