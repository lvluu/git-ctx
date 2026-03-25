package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Run("non-existent file returns defaults", func(t *testing.T) {
		tmp := t.TempDir()
		cfg, err := Load(filepath.Join(tmp, "nonexistent.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ProfilesPath == "" {
			t.Error("expected ProfilesPath to be set")
		}
		if cfg.Worktree.DefaultMode != "symlink" {
			t.Errorf("expected default mode 'symlink', got %s", cfg.Worktree.DefaultMode)
		}
	})
}

func TestInit(t *testing.T) {
	t.Run("creates config file", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.yaml")
		err := Init(path, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) == 0 {
			t.Error("expected config file to have content")
		}
	})

	t.Run("fails if exists", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.yaml")
		os.WriteFile(path, []byte("test"), 0644)
		err := Init(path, false)
		if err == nil {
			t.Error("expected error when file exists")
		}
	})

	t.Run("overwrites with force", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "config.yaml")
		os.WriteFile(path, []byte("test"), 0644)
		err := Init(path, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestMatchDirectoryRule(t *testing.T) {
	cfg := AppConfig{
		DirectoryRules: []DirectoryRule{
			{Pattern: "~/work", Profile: "work"},
			{Pattern: "~/personal", Profile: "personal"},
		},
	}

	t.Run("matches work pattern", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		profile, ok := cfg.MatchDirectoryRule(home + "/work/project")
		if !ok {
			t.Error("expected match")
		}
		if profile != "work" {
			t.Errorf("expected 'work', got %s", profile)
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, ok := cfg.MatchDirectoryRule("/some/other/path")
		if ok {
			t.Error("expected no match")
		}
	})
}
