package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSyncConfig_DefaultsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	cfg, err := loadSyncConfig(filepath.Join(tmp, ".git-ctx-sync.yaml"), "symlink")
	require.NoError(t, err)
	assert.Equal(t, "symlink", cfg.Mode)
	assert.Empty(t, cfg.Files)
}

func TestLoadSyncConfig_ParsesYAML(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx-sync.yaml")
	content := `
mode: copy
files:
  - app/.env
  - .vscode/settings.json
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

	cfg, err := loadSyncConfig(cfgPath, "symlink")
	require.NoError(t, err)
	assert.Equal(t, "copy", cfg.Mode)
	assert.Equal(t, []string{"app/.env", ".vscode/settings.json"}, cfg.Files)
}

func TestLoadSyncConfig_DefaultModeUsedWhenOmitted(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx-sync.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("files:\n  - .env\n"), 0644))

	cfg, err := loadSyncConfig(cfgPath, "copy")
	require.NoError(t, err)
	assert.Equal(t, "copy", cfg.Mode)
}

func TestLoadSyncConfig_InvalidYAMLErrors(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, ".git-ctx-sync.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("files: [\nunclosed"), 0644))

	_, err := loadSyncConfig(cfgPath, "symlink")
	assert.Error(t, err)
}

func TestSyncFiles_SymlinkCreated(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1"), 0644))

	cfg := SyncConfig{Mode: "symlink", Files: []string{".env"}}
	warnings, err := syncFiles(cfg, src, dst, false)
	require.NoError(t, err)
	assert.Empty(t, warnings)

	target, err := os.Readlink(filepath.Join(dst, ".env"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(src, ".env"), target)
}

func TestSyncFiles_CopyCreated(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1"), 0644))

	cfg := SyncConfig{Mode: "copy", Files: []string{".env"}}
	warnings, err := syncFiles(cfg, src, dst, false)
	require.NoError(t, err)
	assert.Empty(t, warnings)

	data, err := os.ReadFile(filepath.Join(dst, ".env"))
	require.NoError(t, err)
	assert.Equal(t, "SECRET=1", string(data))
}

func TestSyncFiles_MissingSourceWarns(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	cfg := SyncConfig{Mode: "symlink", Files: []string{"no-such-file"}}
	warnings, err := syncFiles(cfg, src, dst, false)
	require.NoError(t, err)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "no-such-file")
}

func TestSyncFiles_CreatesParentDirs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested", "dir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "nested", "dir", "config.json"), []byte("{}"), 0644))

	cfg := SyncConfig{Mode: "symlink", Files: []string{"nested/dir/config.json"}}
	warnings, err := syncFiles(cfg, src, dst, false)
	require.NoError(t, err)
	assert.Empty(t, warnings)

	_, err = os.Lstat(filepath.Join(dst, "nested", "dir", "config.json"))
	assert.NoError(t, err)
}

func TestSyncFiles_CopyOverridesModeFlag(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(src, ".env"), []byte("X=1"), 0644))

	cfg := SyncConfig{Mode: "symlink", Files: []string{".env"}}
	warnings, err := syncFiles(cfg, src, dst, true) // copyOverride=true
	require.NoError(t, err)
	assert.Empty(t, warnings)

	info, err := os.Lstat(filepath.Join(dst, ".env"))
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink)
}
