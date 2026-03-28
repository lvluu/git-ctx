package worktree

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSyncConfig(t *testing.T) {
	t.Run("non-existent returns defaults", func(t *testing.T) {
		tmp := t.TempDir()
		cfg, err := LoadSyncConfig(filepath.Join(tmp, "sync.yaml"), "symlink")
		require.NoError(t, err)
		assert.Equal(t, "symlink", cfg.Mode)
		assertEmpty(t, cfg.Files)
	})

	t.Run("loads valid config", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "sync.yaml")
		err := os.WriteFile(path, []byte(`
mode: copy
files:
  - .env
  - .vscode/settings.json
`), 0644)
		require.NoError(t, err)

		cfg, err := LoadSyncConfig(path, "symlink")
		require.NoError(t, err)
		assert.Equal(t, "copy", cfg.Mode)
		assert.Len(t, cfg.Files, 2)
	})

	t.Run("missing mode falls back to default", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "sync.yaml")
		err := os.WriteFile(path, []byte(`files: [.env]`), 0644)
		require.NoError(t, err)

		cfg, err := LoadSyncConfig(path, "copy")
		require.NoError(t, err)
		assert.Equal(t, "copy", cfg.Mode)
	})
}

func TestSyncFiles(t *testing.T) {
	t.Run("skips missing source files", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		cfg := SyncConfig{Mode: "copy", Files: []string{"missing.txt"}}
		warnings, err := SyncFiles(cfg, src, dst, false)
		require.NoError(t, err)
		assert.Len(t, warnings, 1)
		assert.Contains(t, warnings[0], "missing.txt")
	})

	t.Run("copies file when mode is copy", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		srcFile := filepath.Join(src, "config.yaml")
		err := os.WriteFile(srcFile, []byte("hello: world\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "copy", Files: []string{"config.yaml"}}
		warnings, err := SyncFiles(cfg, src, dst, false)
		require.NoError(t, err)
		assert.Empty(t, warnings)

		data, err := os.ReadFile(filepath.Join(dst, "config.yaml"))
		require.NoError(t, err)
		assert.Equal(t, "hello: world\n", string(data))
	})

	t.Run("copyOverride forces copy even in symlink mode", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		srcFile := filepath.Join(src, "config.yaml")
		err := os.WriteFile(srcFile, []byte("hello: world\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "symlink", Files: []string{"config.yaml"}}
		warnings, err := SyncFiles(cfg, src, dst, true) // copyOverride=true
		require.NoError(t, err)
		assert.Empty(t, warnings)

		// Verify it's a regular file, not a symlink.
		info, err := os.Lstat(filepath.Join(dst, "config.yaml"))
		require.NoError(t, err)
		assert.NotEqual(t, os.ModeSymlink, info.Mode()&os.ModeSymlink)
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

func TestSyncDirectionFiles_Push(t *testing.T) {
	t.Run("push syncs files without conflict check", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		// Write different content to src and dst.
		err := os.WriteFile(filepath.Join(src, "shared.txt"), []byte("src version\n"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dst, "shared.txt"), []byte("dst version\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "copy", Files: []string{"shared.txt"}}
		warnings, err := SyncDirectionFiles(cfg, src, dst, Push, false)
		require.NoError(t, err)
		assert.Empty(t, warnings)

		// Push should have overwritten dst without error.
		data, err := os.ReadFile(filepath.Join(dst, "shared.txt"))
		require.NoError(t, err)
		assert.Equal(t, "src version\n", string(data))
	})
}

func TestSyncDirectionFiles_Pull_Conflict(t *testing.T) {
	t.Run("pull detects conflict when both files differ", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		// Write different content with different mtimes (1 second apart).
		err := os.WriteFile(filepath.Join(src, "conflicted.txt"), []byte("src content\n"), 0644)
		require.NoError(t, err)
		time.Sleep(1100 * time.Millisecond)
		err = os.WriteFile(filepath.Join(dst, "conflicted.txt"), []byte("dst content\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "copy", Files: []string{"conflicted.txt"}}
		_, err = SyncDirectionFiles(cfg, src, dst, Pull, false)
		require.Error(t, err)

		var conflictErr *ConflictError
		assert.ErrorAs(t, err, &conflictErr)
		assert.Contains(t, conflictErr.Files, "conflicted.txt")
	})

	t.Run("pull succeeds when only destination changed", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		// Write same content to both.
		err := os.WriteFile(filepath.Join(src, "same.txt"), []byte("same\n"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dst, "same.txt"), []byte("same\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "copy", Files: []string{"same.txt"}}
		_, err = SyncDirectionFiles(cfg, src, dst, Pull, false)
		require.NoError(t, err)
	})

		t.Run("pull succeeds when source is newer and dst unchanged", func(t *testing.T) {
			tmp := t.TempDir()
			src := filepath.Join(tmp, "src")
			dst := filepath.Join(tmp, "dst")
			os.MkdirAll(src, 0755)
			os.MkdirAll(dst, 0755)

			// Write dst FIRST (older), then src (newer) — src must be strictly newer.
			f, err := os.Create(filepath.Join(dst, "newer.txt"))
			require.NoError(t, err)
			f.WriteString("older")
			f.Close()
			dstInfo, _ := os.Stat(filepath.Join(dst, "newer.txt"))

			time.Sleep(2100 * time.Millisecond) // ensure filesystem time advances
			f, err = os.Create(filepath.Join(src, "newer.txt"))
			require.NoError(t, err)
			f.WriteString("newer")
			f.Close()
			srcInfo, _ := os.Stat(filepath.Join(src, "newer.txt"))

			// Assert src is strictly newer so the test is meaningful.
			assert.True(t, srcInfo.ModTime().After(dstInfo.ModTime()),
				"src must be newer than dst for this test; got src=%v dst=%v",
				srcInfo.ModTime(), dstInfo.ModTime())

			cfg := SyncConfig{Mode: "copy", Files: []string{"newer.txt"}}
			_, err = SyncDirectionFiles(cfg, src, dst, Pull, false)
			require.NoError(t, err) // dst was older than src; no conflict

			data, err := os.ReadFile(filepath.Join(dst, "newer.txt"))
			require.NoError(t, err)
			assert.Equal(t, "newer", string(data))
		})


	t.Run("pull succeeds when dst does not exist", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		err := os.WriteFile(filepath.Join(src, "new.txt"), []byte("new content\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "copy", Files: []string{"new.txt"}}
		_, err = SyncDirectionFiles(cfg, src, dst, Pull, false)
		require.NoError(t, err)
	})

		t.Run("pull detects conflict when only mtime differs but content same", func(t *testing.T) {
			tmp := t.TempDir()
			src := filepath.Join(tmp, "src")
			dst := filepath.Join(tmp, "dst")
			os.MkdirAll(src, 0755)
			os.MkdirAll(dst, 0755)

			// Write dst FIRST, then src — src is newer, content is the same (no conflict).
			f, err := os.Create(filepath.Join(dst, "same.txt"))
			require.NoError(t, err)
			f.WriteString("same")
			f.Close()

			time.Sleep(2100 * time.Millisecond)
			f, err = os.Create(filepath.Join(src, "same.txt"))
			require.NoError(t, err)
			f.WriteString("same")
			f.Close()

			cfg := SyncConfig{Mode: "copy", Files: []string{"same.txt"}}
			_, err = SyncDirectionFiles(cfg, src, dst, Pull, false)
			require.NoError(t, err) // same content, no conflict
		})

}

func TestConflictError(t *testing.T) {
	err := &ConflictError{Files: []string{"a.txt", "b.txt"}}
	assert.Contains(t, err.Error(), "sync conflict")
	assert.Contains(t, err.Error(), "a.txt")
	assert.Contains(t, err.Error(), "b.txt")
}

func TestFileHash(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "file1.txt")
	f2 := filepath.Join(tmp, "file2.txt")
	err := os.WriteFile(f1, []byte("hello"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(f2, []byte("hello"), 0644)
	require.NoError(t, err)

	h1, err := fileHash(f1)
	require.NoError(t, err)
	h2, err := fileHash(f2)
	require.NoError(t, err)
	assert.Equal(t, h1, h2)

	h3, err := fileHash(filepath.Join(tmp, "missing.txt"))
	assert.Error(t, err)
	assert.Nil(t, h3)
}

func TestBytesEqual(t *testing.T) {
	assert.True(t, bytesEqual([]byte("hello"), []byte("hello")))
	assert.False(t, bytesEqual([]byte("hello"), []byte("world")))
	assert.False(t, bytesEqual([]byte("hello"), []byte("hell")))
	assert.False(t, bytesEqual([]byte{}, []byte("x")))
}

func TestAbs(t *testing.T) {
	assert.Equal(t, time.Duration(5), abs(time.Duration(5)))
	assert.Equal(t, time.Duration(5), abs(time.Duration(-5)))
}

func TestLoadSyncConfigForPath(t *testing.T) {
	t.Run("missing file returns error", func(t *testing.T) {
		tmp := t.TempDir()
		_, err := loadSyncConfigForPath(filepath.Join(tmp, "notexist.yaml"), "symlink")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no .git-ctx-sync.yaml")
	})

	t.Run("valid file parsed correctly", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, ".git-ctx-sync.yaml")
		err := os.WriteFile(path, []byte("mode: copy\nfiles:\n  - .env\n"), 0644)
		require.NoError(t, err)

		cfg, err := loadSyncConfigForPath(path, "symlink")
		require.NoError(t, err)
		assert.Equal(t, "copy", cfg.Mode)
	})
}

func TestSyncDirectionFiles_Symlink(t *testing.T) {
	t.Run("push creates symlinks", func(t *testing.T) {
		tmp := t.TempDir()
		src := filepath.Join(tmp, "src")
		dst := filepath.Join(tmp, "dst")
		os.MkdirAll(src, 0755)
		os.MkdirAll(dst, 0755)

		err := os.WriteFile(filepath.Join(src, "linked.txt"), []byte("content\n"), 0644)
		require.NoError(t, err)

		cfg := SyncConfig{Mode: "symlink", Files: []string{"linked.txt"}}
		_, err = SyncDirectionFiles(cfg, src, dst, Push, false)
		require.NoError(t, err)

		link := filepath.Join(dst, "linked.txt")
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.True(t, info.Mode()&os.ModeSymlink != 0)
	})
}

func assertEmpty(t *testing.T, s []string) {
	t.Helper()
	if len(s) != 0 {
		t.Errorf("expected empty slice, got %d items", len(s))
	}
}
