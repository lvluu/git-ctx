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
}
