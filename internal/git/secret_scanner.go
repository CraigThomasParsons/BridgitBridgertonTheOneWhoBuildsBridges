// Package git — secret_scanner.go provides a pre-commit guardrail that detects
// files containing secrets before they are staged. Used by the provisioning phase
// to prevent leaking API keys, tokens, and credentials to GitHub.
//
// Scanning happens at the file level (filepath matching) and at the content
// level (regex pattern matching). Files that match either check are excluded
// from staging and reported to the caller.
package git

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SecretFinding records a single detected secret in a file. Each finding
// includes the file path, which rule matched, and the line number if the
// match was content-based (zero for filename-based matches).
type SecretFinding struct {
	// FilePath is the path to the file containing the potential secret.
	FilePath string

	// Rule describes which pattern matched (e.g., "filename:.env", "pattern:AWS key").
	Rule string

	// LineNumber is the source line where the secret was found. Zero for
	// filename-based matches where the entire file is flagged.
	LineNumber int
}

// ScanForSecrets walks a directory tree and checks every file against known
// secret patterns. Returns a slice of findings (empty if clean) and the list
// of files that should be excluded from git staging. Skips the .git directory
// and any files already listed in .gitignore.
func ScanForSecrets(rootPath string) ([]SecretFinding, []string) {
	var allFindings []SecretFinding
	var excludedFiles []string

	// Walk the directory tree, skipping .git and common ignore directories.
	_ = filepath.Walk(rootPath, func(filePath string, fileInfo os.FileInfo, walkError error) error {
		if walkError != nil {
			return nil
		}

		// Skip the .git directory entirely — it's not user content.
		if fileInfo.IsDir() && fileInfo.Name() == ".git" {
			return filepath.SkipDir
		}

		// Skip directories — we only scan files.
		if fileInfo.IsDir() {
			return nil
		}

		// Get the path relative to the root for cleaner output.
		relativePath, relError := filepath.Rel(rootPath, filePath)
		if relError != nil {
			relativePath = filePath
		}

		// Check filename-based rules first (fast, no I/O needed).
		filenameRule := checkFilenameRules(relativePath)
		if filenameRule != "" {
			allFindings = append(allFindings, SecretFinding{
				FilePath:   relativePath,
				Rule:       filenameRule,
				LineNumber: 0,
			})
			excludedFiles = append(excludedFiles, relativePath)
			return nil
		}

		// Skip binary files and large files (over 1MB) for content scanning.
		if fileInfo.Size() > 1024*1024 {
			return nil
		}

		// Scan file contents for secret patterns.
		contentFindings := scanFileContents(filePath, relativePath)
		if len(contentFindings) > 0 {
			allFindings = append(allFindings, contentFindings...)
			excludedFiles = append(excludedFiles, relativePath)
		}

		return nil
	})

	return allFindings, excludedFiles
}

// dangerousFilenames are files that should never be committed regardless of
// their content. These are always flagged by the scanner.
var dangerousFilenames = []string{
	".env",
	".env.local",
	".env.production",
	".env.development",
	".env.staging",
	"credentials.json",
	"service-account.json",
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
}

// dangerousExtensions are file extensions that typically contain secrets
// or private key material. Matched case-insensitively.
var dangerousExtensions = []string{
	".pem",
	".key",
	".p12",
	".pfx",
	".jks",
	".keystore",
}

// checkFilenameRules tests whether a file path matches any known dangerous
// filename or extension pattern. Returns the matched rule name, or empty
// string if no match was found.
func checkFilenameRules(relativePath string) string {
	// Extract just the filename for exact-match checks.
	baseName := filepath.Base(relativePath)
	lowerBaseName := strings.ToLower(baseName)

	// Check against known dangerous filenames.
	for _, dangerousName := range dangerousFilenames {
		if lowerBaseName == dangerousName {
			return "filename:" + dangerousName
		}
	}

	// Check for dangerous extensions.
	lowerExt := strings.ToLower(filepath.Ext(baseName))
	for _, dangerousExt := range dangerousExtensions {
		if lowerExt == dangerousExt {
			return "extension:" + dangerousExt
		}
	}

	return ""
}

// secretPatterns are compiled regexes that match common secret formats found
// in source code and configuration files. Each pattern is paired with a
// human-readable rule name for reporting.
var secretPatterns = []struct {
	// RuleName is a short label describing what this pattern catches.
	RuleName string

	// Pattern is the compiled regex used to scan each line.
	Pattern *regexp.Regexp
}{
	{"OpenAI API key", regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)},
	{"Anthropic API key", regexp.MustCompile(`sk-ant-[a-zA-Z0-9\-_]{20,}`)},
	{"GitHub token (classic)", regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`)},
	{"GitHub PAT (fine-grained)", regexp.MustCompile(`github_pat_[a-zA-Z0-9_]{20,}`)},
	{"GitHub OAuth", regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`)},
	{"AWS access key", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"Google API key", regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`)},
	{"Groq API key", regexp.MustCompile(`gsk_[a-zA-Z0-9]{20,}`)},
	{"Slack token", regexp.MustCompile(`xox[bpors]-[0-9a-zA-Z\-]{10,}`)},
	{"Private key header", regexp.MustCompile(`-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)},
	{"Bearer token in code", regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-_.]{20,}`)},
	{"Generic secret assignment", regexp.MustCompile(`(?i)(api_key|apikey|secret_key|private_key|access_token|auth_token)\s*[=:]\s*['"][a-zA-Z0-9\-_.]{16,}['"]`)},
}

// scanFileContents reads a file line by line and checks each line against
// all known secret patterns. Returns findings for any matches found.
func scanFileContents(absolutePath string, relativePath string) []SecretFinding {
	// Open the file for reading — skip if it cannot be opened.
	fileHandle, openError := os.Open(absolutePath)
	if openError != nil {
		return nil
	}
	defer fileHandle.Close()

	var contentFindings []SecretFinding
	lineScanner := bufio.NewScanner(fileHandle)
	currentLine := 0

	// Scan each line against all secret patterns.
	for lineScanner.Scan() {
		currentLine++
		lineText := lineScanner.Text()

		// Test each pattern against the current line.
		for _, secretRule := range secretPatterns {
			if secretRule.Pattern.MatchString(lineText) {
				contentFindings = append(contentFindings, SecretFinding{
					FilePath:   relativePath,
					Rule:       "pattern:" + secretRule.RuleName,
					LineNumber: currentLine,
				})
				// Only report the first pattern match per line to avoid noise.
				break
			}
		}
	}

	return contentFindings
}
