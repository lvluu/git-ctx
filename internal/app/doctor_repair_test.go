package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureShellHook_AlreadyPresent(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, "bashrc")
	snippet := ShellInitScript()

	// Write content that already contains the git-ctx init.
	content := `existing config
eval "$(git ctx shell-init)"
alias gc="git-ctx"
more config
`
	os.WriteFile(rc, []byte(content), 0644)

	changed, backup, err := ensureShellHook(rc, snippet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Error("expected no change when already present")
	}
	if backup != "" {
		t.Errorf("expected no backup, got %s", backup)
	}

	// Verify file unchanged.
	data, _ := os.ReadFile(rc)
	if string(data) != content {
		t.Error("file should not have been modified")
	}
}

func TestEnsureShellHook_Appends(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, "bashrc")
	os.WriteFile(rc, []byte("existing config\n"), 0644)
	snippet := ShellInitScript()

	changed, backup, err := ensureShellHook(rc, snippet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected change")
	}
	if backup == "" {
		t.Error("expected backup path")
	}

	// Verify backup exists and has original content.
	backupData, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(backupData) != "existing config\n" {
		t.Error("backup should contain original content")
	}

	// Verify snippet was appended.
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), "alias gc=\"git-ctx\"") {
		t.Error("snippet not appended")
	}
}

func TestEnsureShellHook_DryRun(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, "bashrc")
	os.WriteFile(rc, []byte("existing\n"), 0644)
	snippet := ShellInitScript()

	changed, backup, err := ensureShellHook(rc, snippet, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected would-change in dry run")
	}
	if backup != "" {
		t.Error("expected no backup in dry run")
	}

	// Verify file was NOT modified.
	data, _ := os.ReadFile(rc)
	if string(data) != "existing\n" {
		t.Error("file should not be modified in dry run")
	}
}

func TestEnsureShellHook_CreatesNewFile(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, "newbashrc")
	// File does not exist.
	snippet := ShellInitScript()

	changed, backup, err := ensureShellHook(rc, snippet, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected change when creating new file")
	}
	if backup != "" {
		t.Error("expected no backup for new file")
	}

	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if !strings.Contains(string(data), "alias gc=\"git-ctx\"") {
		t.Error("snippet not in new file")
	}
}

func TestEnsureShellHook_DryRunNewFile(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, "newbashrc")
	snippet := ShellInitScript()

	changed, backup, err := ensureShellHook(rc, snippet, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected would-change in dry run for new file")
	}
	if backup != "" {
		t.Error("expected no backup in dry run")
	}
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Error("file should not be created in dry run")
	}
}

func TestFixProfilesFile_CreatesNew(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")

	result, err := fixProfilesFile(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file should be created")
	}

	var store struct {
		Profiles map[string]any `json:"profiles"`
	}
	data, _ := os.ReadFile(path)
	if err := json.Unmarshal(data, &store); err != nil {
		t.Errorf("file is not valid JSON: %v", err)
	}
}

func TestFixProfilesFile_DryRunNew(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")

	result, err := fixProfilesFile(path, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if len(result.DryRunHints) == 0 {
		t.Error("expected dry-run hints")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not be created in dry run")
	}
}

func TestFixProfilesFile_ValidJSON_Skipped(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	os.WriteFile(path, []byte(`{"profiles":{"work":{"name":"Test","email":"test@example.com"}}}`), 0644)

	result, err := fixProfilesFile(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success (skip)")
	}
	if !strings.Contains(result.Summary, "already valid") {
		t.Errorf("expected 'already valid', got: %s", result.Summary)
	}
}

func TestFixProfilesFile_LegacyFormat_Skipped(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	os.WriteFile(path, []byte(`{"work":{"name":"Test","email":"test@example.com"}}`), 0644)

	result, err := fixProfilesFile(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success (skip)")
	}
	if !strings.Contains(result.Summary, "legacy format") {
		t.Errorf("expected 'legacy format', got: %s", result.Summary)
	}
}

func TestFixProfilesFile_Corrupted_BacksUpAndFixes(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	os.WriteFile(path, []byte(`{not valid json`), 0644)

	result, err := fixProfilesFile(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.BackupPath == "" {
		t.Error("expected backup path")
	}

	// Verify backup has corrupted content.
	backupData, _ := os.ReadFile(result.BackupPath)
	if string(backupData) != `{not valid json` {
		t.Error("backup should contain original corrupted content")
	}

	// Verify repaired file is valid and empty.
	data, _ := os.ReadFile(path)
	var store struct {
		Profiles map[string]any `json:"profiles"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		t.Errorf("repaired file is not valid JSON: %v", err)
	}
}

func TestFixProfilesFile_Corrupted_DryRun(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	os.WriteFile(path, []byte(`{broken`), 0644)

	result, err := fixProfilesFile(path, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if len(result.DryRunHints) == 0 {
		t.Error("expected dry-run hints")
	}
	// Original file should be unchanged.
	data, _ := os.ReadFile(path)
	if string(data) != `{broken` {
		t.Error("file should not be modified in dry run")
	}
}

func TestExtractProfileName(t *testing.T) {
	tests := []struct {
		detail  string
		want    string
		wantErr bool
	}{
		{
			detail:  "profile 'work' not found in /home/user/.git-ctx-profiles.json",
			want:    "work",
			wantErr: false,
		},
		{
			detail:  "profile 'personal' not found",
			want:    "personal",
			wantErr: false,
		},
		{
			detail:  "no profile mentioned here",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		got := extractProfileName(tt.detail)
		if got != tt.want {
			t.Errorf("extractProfileName(%q) = %q, want %q", tt.detail, got, tt.want)
		}
	}
}

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")

	content := "hello world"
	os.WriteFile(src, []byte(content), 0644)

	err := copyFile(dst, src)
	if err != nil {
		t.Fatalf("copyFile error: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != content {
		t.Errorf("dst content = %q, want %q", string(data), content)
	}
}

func TestCopyFile_SourceNotFound(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "dst")

	err := copyFile(dst, filepath.Join(tmp, "nonexistent"))
	if err == nil {
		t.Error("expected error when source not found")
	}
}
