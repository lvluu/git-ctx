package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoResolver_DirectoryRuleFallback(t *testing.T) {
	home := t.TempDir()
	workDir := filepath.Join(home, "work", "myrepo")
	require.NoError(t, os.MkdirAll(workDir, 0755))

	rules := []DirectoryRule{
		{Pattern: filepath.Join(home, "work"), Profile: "work"},
		{Pattern: filepath.Join(home, "personal"), Profile: "personal"},
	}

	resolver := AutoResolver{
		GetRepoRoot: func() (string, bool, error) { return workDir, true, nil },
		GetHomeDir:  func() (string, error) { return home, nil },
		ReadFile:    os.ReadFile,
		FileExists:  func(path string) bool { _, err := os.Stat(path); return err == nil },
		DirectoryRules: rules,
		GetCurrentDir:  func() (string, error) { return workDir, nil },
	}

	res, err := resolver.Resolve()
	require.NoError(t, err)
	assert.Equal(t, "work", res.ProfileKey)
	assert.Equal(t, "--local", res.ScopeFlag)
}

func TestAutoResolver_DirectoryRuleNoMatch(t *testing.T) {
	home := t.TempDir()
	otherDir := filepath.Join(home, "other", "proj")
	require.NoError(t, os.MkdirAll(otherDir, 0755))

	resolver := AutoResolver{
		GetRepoRoot: func() (string, bool, error) { return "", false, nil },
		GetHomeDir:  func() (string, error) { return home, nil },
		ReadFile:    os.ReadFile,
		FileExists:  func(path string) bool { _, err := os.Stat(path); return err == nil },
		DirectoryRules: []DirectoryRule{
			{Pattern: filepath.Join(home, "work"), Profile: "work"},
		},
		GetCurrentDir: func() (string, error) { return otherDir, nil },
	}

	_, err := resolver.Resolve()
	assert.Error(t, err)
}

func TestShellInitOutput_ContainsBashHook(t *testing.T) {
	out := shellInitScript()
	assert.Contains(t, out, "PROMPT_COMMAND")
	assert.Contains(t, out, "__git_ctx_auto")
	assert.Contains(t, out, "profile auto --silent")
	assert.Contains(t, out, `alias gc="git-ctx"`)
}

func TestShellInitOutput_ContainsZshHook(t *testing.T) {
	out := shellInitScript()
	assert.Contains(t, out, "ZSH_VERSION")
	assert.Contains(t, out, "add-zsh-hook")
}

func TestAutoResolver_DirectoryRuleGlobalWhenNotInRepo(t *testing.T) {
	home := t.TempDir()
	workDir := filepath.Join(home, "work", "proj")
	require.NoError(t, os.MkdirAll(workDir, 0755))

	resolver := AutoResolver{
		GetRepoRoot: func() (string, bool, error) { return "", false, nil },
		GetHomeDir:  func() (string, error) { return home, nil },
		ReadFile:    os.ReadFile,
		FileExists:  func(path string) bool { _, err := os.Stat(path); return err == nil },
		DirectoryRules: []DirectoryRule{
			{Pattern: filepath.Join(home, "work"), Profile: "work"},
		},
		GetCurrentDir: func() (string, error) { return workDir, nil },
	}

	res, err := resolver.Resolve()
	require.NoError(t, err)
	assert.Equal(t, "work", res.ProfileKey)
	assert.Equal(t, "--global", res.ScopeFlag) // not in repo → global
	assert.Equal(t, "", res.WorkDir)
}
