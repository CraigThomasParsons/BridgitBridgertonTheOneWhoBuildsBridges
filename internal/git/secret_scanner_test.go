package git

import (
	"os"
	"testing"
)

// TestScanForSecrets_DetectsEnvFile verifies the scanner catches .env files
// by scanning a temporary directory containing a mock .env file.
func TestScanForSecrets_DetectsEnvFile(t *testing.T) {
	// Create a temp directory with a fake .env file to scan.
	tempDir := t.TempDir()

	// Write a mock .env containing a known secret pattern.
	mockEnvContent := "CHATGPT_SESSION_TOKEN='eyJhbGciOiJkaXIiLCJlbmMiOi...'\n"
	if err := writeTestFile(tempDir+"/.env", mockEnvContent); err != nil {
		t.Fatalf("failed to write mock .env: %v", err)
	}

	// Write a safe file that should NOT be flagged.
	safeContent := "package main\n\nfunc main() {}\n"
	if err := writeTestFile(tempDir+"/main.go", safeContent); err != nil {
		t.Fatalf("failed to write safe file: %v", err)
	}

	// Run the scanner against the temp directory.
	findings, excluded := ScanForSecrets(tempDir)

	// Verify .env was caught by the filename rule.
	if len(findings) == 0 {
		t.Fatal("expected at least one finding, got zero")
	}
	if findings[0].Rule != "filename:.env" {
		t.Errorf("expected rule 'filename:.env', got '%s'", findings[0].Rule)
	}
	if len(excluded) != 1 || excluded[0] != ".env" {
		t.Errorf("expected excluded=[.env], got %v", excluded)
	}
}

// TestScanForSecrets_DetectsAPIKeys verifies content-based pattern matching
// catches common API key formats embedded in source files.
func TestScanForSecrets_DetectsAPIKeys(t *testing.T) {
	tempDir := t.TempDir()

	// Write a config file containing a Groq API key pattern.
	configContent := `API_KEY = "gsk_C3qFhfPRqPBkHsU14RBSWGdyb3FY"` + "\n"
	if err := writeTestFile(tempDir+"/config.toml", configContent); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	findings, excluded := ScanForSecrets(tempDir)

	if len(findings) == 0 {
		t.Fatal("expected findings for Groq API key pattern, got zero")
	}
	if len(excluded) != 1 || excluded[0] != "config.toml" {
		t.Errorf("expected excluded=[config.toml], got %v", excluded)
	}
}

// TestScanForSecrets_CleanDirectory verifies no false positives on safe files.
func TestScanForSecrets_CleanDirectory(t *testing.T) {
	tempDir := t.TempDir()

	safeContent := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := writeTestFile(tempDir+"/main.go", safeContent); err != nil {
		t.Fatalf("failed to write safe file: %v", err)
	}

	findings, excluded := ScanForSecrets(tempDir)

	if len(findings) != 0 {
		t.Errorf("expected zero findings for clean dir, got %d: %v", len(findings), findings)
	}
	if len(excluded) != 0 {
		t.Errorf("expected zero excluded for clean dir, got %v", excluded)
	}
}

// writeTestFile is a helper that creates a file with the given content.
func writeTestFile(filePath string, content string) error {
	return os.WriteFile(filePath, []byte(content), 0644)
}
