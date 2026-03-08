package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAppConfig_DefaultsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := loadAppConfig(filepath.Join(tmp, "no-such.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "symlink", cfg.Worktree.DefaultMode)
}

func TestLoadAppConfig_ParsesYAML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx.yaml")
	content := `
worktree:
  default_mode: copy
profiles_path: /custom/profiles.json
directory_rules:
  - pattern: "~/work"
    profile: work
  - pattern: "~/personal"
    profile: personal
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

	cfg, err := loadAppConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "copy", cfg.Worktree.DefaultMode)
	assert.Equal(t, "/custom/profiles.json", cfg.ProfilesPath)
	assert.Len(t, cfg.DirectoryRules, 2)
	assert.Equal(t, "work", cfg.DirectoryRules[0].Profile)
}

func TestLoadAppConfig_InvalidYAMLErrors(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("profiles_path: [\nunclosed"), 0644))
	_, err := loadAppConfig(cfgPath)
	assert.Error(t, err)
}

func TestInitAppConfig_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx.yaml")

	err := initAppConfig(cfgPath, false)
	require.NoError(t, err)

	_, err = os.Stat(cfgPath)
	assert.NoError(t, err)

	cfg, err := loadAppConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "symlink", cfg.Worktree.DefaultMode)
}

func TestInitAppConfig_NoOverwriteWithoutForce(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("profiles_path: /old\n"), 0644))

	err := initAppConfig(cfgPath, false)
	assert.ErrorContains(t, err, "already exists")
}

func TestInitAppConfig_OverwriteWithForce(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("profiles_path: /old\n"), 0644))

	err := initAppConfig(cfgPath, true)
	assert.NoError(t, err)
}

func TestMatchDirectoryRule_LongestPrefixWins(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := AppConfig{
		DirectoryRules: []DirectoryRule{
			{Pattern: filepath.Join(home, "work"), Profile: "work"},
			{Pattern: filepath.Join(home, "work", "client-a"), Profile: "client-a"},
			{Pattern: filepath.Join(home, "personal"), Profile: "personal"},
		},
	}

	p, ok := cfg.MatchDirectoryRule(filepath.Join(home, "work", "client-a", "myrepo"))
	assert.True(t, ok)
	assert.Equal(t, "client-a", p)

	p, ok = cfg.MatchDirectoryRule(filepath.Join(home, "work", "internal"))
	assert.True(t, ok)
	assert.Equal(t, "work", p)

	p, ok = cfg.MatchDirectoryRule(filepath.Join(home, "personal", "blog"))
	assert.True(t, ok)
	assert.Equal(t, "personal", p)

	_, ok = cfg.MatchDirectoryRule(filepath.Join(home, "other"))
	assert.False(t, ok)
}

func TestMatchDirectoryRule_TildeExpanded(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := AppConfig{
		DirectoryRules: []DirectoryRule{
			{Pattern: "~/work", Profile: "work"},
		},
	}
	p, ok := cfg.MatchDirectoryRule(filepath.Join(home, "work", "myrepo"))
	assert.True(t, ok)
	assert.Equal(t, "work", p)
}
