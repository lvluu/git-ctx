package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDoctorChecks_AllPass(t *testing.T) {
	tmp := t.TempDir()

	profilesPath := filepath.Join(tmp, "profiles.json")
	_ = os.WriteFile(profilesPath, []byte(`{"work":{"name":"A","email":"a@b.com"}}`), 0644)

	cfg := AppConfig{
		ProfilesPath:   profilesPath,
		DirectoryRules: []DirectoryRule{{Pattern: filepath.Join(tmp, "work"), Profile: "work"}},
		Worktree:       AppWorktreeConfig{DefaultMode: "symlink"},
	}

	results := runDoctorChecks(cfg)
	for _, r := range results {
		assert.True(t, r.OK, "check %q failed: %s", r.Name, r.Detail)
	}
}

func TestDoctorChecks_MissingProfilesFile(t *testing.T) {
	cfg := AppConfig{
		ProfilesPath: "/nonexistent/profiles.json",
		Worktree:     AppWorktreeConfig{DefaultMode: "symlink"},
	}
	results := runDoctorChecks(cfg)
	found := false
	for _, r := range results {
		if r.Name == "profiles file" {
			found = true
			assert.NotEmpty(t, r.Detail)
		}
	}
	assert.True(t, found, "profiles file check not present")
}

func TestDoctorChecks_InvalidWorktreeMode(t *testing.T) {
	cfg := AppConfig{
		Worktree: AppWorktreeConfig{DefaultMode: "invalid"},
	}
	results := runDoctorChecks(cfg)
	for _, r := range results {
		if r.Name == "worktree mode" {
			assert.False(t, r.OK)
			assert.Contains(t, r.Detail, "invalid")
			return
		}
	}
	t.Fatal("worktree mode check not found")
}

func TestDoctorChecks_DirectoryRuleProfileMissing(t *testing.T) {
	tmp := t.TempDir()

	profilesPath := filepath.Join(tmp, "profiles.json")
	_ = os.WriteFile(profilesPath, []byte(`{"work":{"name":"A","email":"a@b.com"}}`), 0644)

	cfg := AppConfig{
		ProfilesPath: profilesPath,
		DirectoryRules: []DirectoryRule{
			{Pattern: filepath.Join(tmp, "work"), Profile: "work"},
			{Pattern: filepath.Join(tmp, "other"), Profile: "missing-profile"},
		},
		Worktree: AppWorktreeConfig{DefaultMode: "symlink"},
	}

	results := runDoctorChecks(cfg)
	failCount := 0
	for _, r := range results {
		if !r.OK {
			failCount++
		}
	}
	assert.Equal(t, 1, failCount, "expected exactly 1 failed check (missing profile in directory rule)")
}

func TestPrintDoctorResults_NoOutput(t *testing.T) {
	// Just ensure printDoctorResults doesn't panic
	results := []DoctorResult{
		{Name: "test", OK: true, Detail: "ok"},
		{Name: "fail", OK: false, Detail: "problem"},
	}
	// No assertion needed — just verify no panic
	printDoctorResults(results)
}
