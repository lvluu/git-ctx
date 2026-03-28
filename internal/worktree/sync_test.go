package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSyncConfig(t *testing.T) {
	t.Run("non-existent returns defaults", func(t *testing.T) {
		tmp := t.TempDir()
		cfg, err := LoadSyncConfig(filepath.Join(tmp, "sync.yaml"), "symlink")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Mode != "symlink" {
			t.Errorf("expected 'symlink', got %s", cfg.Mode)
		}
		if len(cfg.Files) != 0 {
			t.Errorf("expected 0 files, got %d", len(cfg.Files))
		}
	})

	t.Run("loads valid config", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "sync.yaml")
		os.WriteFile(path, []byte(`
mode: copy
files:
  - .env
  - .vscode/settings.json
`), 0644)

		cfg, err := LoadSyncConfig(path, "symlink")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Mode != "copy" {
			t.Errorf("expected 'copy', got %s", cfg.Mode)
		}
		if len(cfg.Files) != 2 {
			t.Errorf("expected 2 files, got %d", len(cfg.Files))
		}
	})
}

func TestSyncFiles(t *testing.T) {
	t.Run("skips missing source files", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		cfg := SyncConfig{
			Mode:  "copy",
			Files: []string{"missing.txt"},
		}

		warnings, err := SyncFiles(cfg, src, dst, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 1 {
			t.Errorf("expected 1 warning, got %d", len(warnings))
		}
	})

	t.Run("push direction: primary → worktree", func(t *testing.T) {
		tmp := t.TempDir()
		primary := filepath.Join(tmp, "primary")
		worktree := filepath.Join(tmp, "worktree")
		os.MkdirAll(primary, 0755)
		os.MkdirAll(worktree, 0755)

		cfgPath := filepath.Join(primary, ".git-ctx-sync.yaml")
		os.WriteFile(cfgPath, []byte(`
mode: copy
files:
  - config.toml
`), 0644)

		os.WriteFile(filepath.Join(primary, "config.toml"), []byte("primary content"), 0644)

		cfg, err := LoadSyncConfig(cfgPath, "copy")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Primary → worktree (push direction).
		warnings, err := SyncFiles(cfg, primary, worktree, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 0 {
			t.Errorf("expected no warnings, got %d: %v", len(warnings), warnings)
		}

		data, err := os.ReadFile(filepath.Join(worktree, "config.toml"))
		if err != nil {
			t.Fatalf("file not synced: %v", err)
		}
		if string(data) != "primary content" {
			t.Errorf("expected 'primary content', got %s", string(data))
		}
	})

	t.Run("pull direction: worktree → primary", func(t *testing.T) {
		tmp := t.TempDir()
		primary := filepath.Join(tmp, "primary")
		worktree := filepath.Join(tmp, "worktree")
		os.MkdirAll(primary, 0755)
		os.MkdirAll(worktree, 0755)

		cfgPath := filepath.Join(primary, ".git-ctx-sync.yaml")
		os.WriteFile(cfgPath, []byte(`
mode: copy
files:
  - config.toml
`), 0644)

		// File originates in worktree.
		os.WriteFile(filepath.Join(worktree, "config.toml"), []byte("worktree content"), 0644)

		cfg, err := LoadSyncConfig(cfgPath, "copy")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Worktree → primary (pull direction).
		_, err = SyncFiles(cfg, worktree, primary, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(primary, "config.toml"))
		if err != nil {
			t.Fatalf("file not pulled: %v", err)
		}
		if string(data) != "worktree content" {
			t.Errorf("expected 'worktree content', got %s", string(data))
		}
	})

	t.Run("resync updates destination", func(t *testing.T) {
		tmp := t.TempDir()
		primary := filepath.Join(tmp, "primary")
		worktree := filepath.Join(tmp, "worktree")
		os.MkdirAll(primary, 0755)
		os.MkdirAll(worktree, 0755)

		cfgPath := filepath.Join(primary, ".git-ctx-sync.yaml")
		os.WriteFile(cfgPath, []byte(`
mode: copy
files:
  - config.toml
`), 0644)

		os.WriteFile(filepath.Join(primary, "config.toml"), []byte("original"), 0644)

		cfg, err := LoadSyncConfig(cfgPath, "copy")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Initial sync.
		_, err = SyncFiles(cfg, primary, worktree, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Edit primary.
		os.WriteFile(filepath.Join(primary, "config.toml"), []byte("updated"), 0644)

		// Re-sync (should overwrite worktree copy).
		_, err = SyncFiles(cfg, primary, worktree, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(worktree, "config.toml"))
		if err != nil {
			t.Fatalf("file not updated: %v", err)
		}
		if string(data) != "updated" {
			t.Errorf("expected 'updated', got %s", string(data))
		}
	})
}
