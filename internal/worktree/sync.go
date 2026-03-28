// Package worktree handles git worktree operations.
package worktree

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/lvluu/git-ctx/internal/config"
	"github.com/lvluu/git-ctx/internal/git"
	"gopkg.in/yaml.v3"
)

// Hooks holds hook commands for worktree lifecycle events.
type Hooks struct {
	PostCreate []string `yaml:"post_create"`
}

// SyncConfig is the per-repo worktree sync configuration from .git-ctx-sync.yaml.
type SyncConfig struct {
	Mode  string   `yaml:"mode"`
	Files []string `yaml:"files"`
	Hooks Hooks    `yaml:"hooks"`
}

// LoadSyncConfig reads .git-ctx-sync.yaml from cfgPath.
// If the file does not exist, an empty config with defaultMode is returned.
func LoadSyncConfig(cfgPath string, defaultMode string) (SyncConfig, error) {
	cfg := SyncConfig{Mode: defaultMode}

	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", cfgPath, err)
	}

	if cfg.Mode == "" {
		cfg.Mode = defaultMode
	}

	return cfg, nil
}

// SyncFiles creates symlinks or copies for each file in cfg.Files from srcRoot to dstRoot.
// copyOverride=true forces copy regardless of cfg.Mode.
func SyncFiles(cfg SyncConfig, srcRoot, dstRoot string, copyOverride bool) (warnings []string, err error) {
	useCopy := copyOverride || cfg.Mode == "copy"

	for _, rel := range cfg.Files {
		src := filepath.Join(srcRoot, rel)
		dst := filepath.Join(dstRoot, rel)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("skipping %s: not found in source worktree", rel))
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return warnings, fmt.Errorf("creating parent dir for %s: %w", rel, err)
		}

		if useCopy {
			if err := copyFile(src, dst); err != nil {
				return warnings, fmt.Errorf("copying %s: %w", rel, err)
			}
		} else {
			// Always use absolute path for symlink target.
			absTarget, err := filepath.Abs(src)
			if err != nil {
				return warnings, fmt.Errorf("resolving absolute path for %s: %w", rel, err)
			}
			// Remove existing symlink/file at dst before creating.
			_ = os.Remove(dst)
			if err := os.Symlink(absTarget, dst); err != nil {
				return warnings, fmt.Errorf("symlinking %s: %w", rel, err)
			}
		}
	}

	return warnings, nil
}

// ListWorktreePaths returns absolute paths of all worktrees.
func ListWorktreePaths(g git.Runner) ([]string, error) {
	out, err := g.Output("", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}

// RunSync loads .git-ctx-sync.yaml from the repo root and syncs files to dstWorktree.
func RunSync(appCfg config.AppConfig, g git.Runner, dstWorktree string, copyOverride bool) ([]string, error) {
	repoRoot, inRepo, err := git.FindRepoRoot(g)
	if err != nil {
		return nil, err
	}
	if !inRepo {
		return nil, fmt.Errorf("not inside a git repository")
	}

	syncCfgPath := filepath.Join(repoRoot, ".git-ctx-sync.yaml")
	cfg, err := LoadSyncConfig(syncCfgPath, appCfg.Worktree.DefaultMode)
	if err != nil {
		return nil, err
	}

	if len(cfg.Files) == 0 {
		return nil, nil
	}

	return SyncFiles(cfg, repoRoot, dstWorktree, copyOverride)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = out.ReadFrom(in)
	return err
}

// SyncDirection specifies the direction of a sync operation.
type SyncDirection string

const (
	// Push syncs files from the primary worktree to other worktrees.
	Push SyncDirection = "push"
	// Pull syncs files from a non-primary worktree back to the primary.
	Pull SyncDirection = "pull"
)

// ErrConflict is returned when a file has been modified in both source and
// destination worktrees since the last sync.
var ErrConflict = errors.New("sync conflict detected")

// ConflictError reports which files conflicted.
type ConflictError struct {
	Files []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("sync conflict: %d file(s) modified in both worktrees: %s",
		len(e.Files), strings.Join(e.Files, ", "))
}

// fileHash returns a SHA-256 hash of the file contents at path.
func fileHash(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// hasConflict checks whether a file has been modified in both src and dst since
// the last sync by comparing content hashes. A conflict exists when both files
// differ and the destination was modified at or after the source (within a 1-second
// window to account for clock imprecision). Returns the conflicting files.
func hasConflict(src, dst string) ([]string, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return nil, err
	}

	dstInfo, err := os.Stat(dst)
	if err != nil {
		// Destination missing — no conflict.
		return nil, nil
	}

	diff := dstInfo.ModTime().Sub(srcInfo.ModTime())

	// If dst was modified strictly before src (by more than 1 second), no conflict.
	if diff < -time.Second {
		return nil, nil
	}

	// dst was modified at or after src — content may have diverged.
	srcHash, err := fileHash(src)
	if err != nil {
		return nil, err
	}
	dstHash, err := fileHash(dst)
	if err != nil {
		return nil, err
	}

	if !bytesEqual(srcHash, dstHash) {
		return []string{filepath.Base(src)}, nil
	}
	return nil, nil
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SyncDirectionFiles syncs files in the given direction. Pull checks for
// conflicts before overwriting; Push overwrites without conflict checks.
func SyncDirectionFiles(cfg SyncConfig, srcRoot, dstRoot string, direction SyncDirection, copyOverride bool) (warnings []string, err error) {
	useCopy := copyOverride || cfg.Mode == "copy"

	for _, rel := range cfg.Files {
		src := filepath.Join(srcRoot, rel)
		dst := filepath.Join(dstRoot, rel)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("skipping %s: not found in source worktree", rel))
			continue
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return warnings, fmt.Errorf("creating parent dir for %s: %w", rel, err)
		}

		// For pull operations, detect conflicts (file changed in both worktrees).
		if direction == Pull {
			conflicts, err := hasConflict(src, dst)
			if err != nil && !os.IsNotExist(err) {
				return warnings, fmt.Errorf("checking conflict for %s: %w", rel, err)
			}
			if len(conflicts) > 0 {
				return warnings, &ConflictError{Files: conflicts}
			}
		}

		if useCopy {
			if err := copyFile(src, dst); err != nil {
				return warnings, fmt.Errorf("copying %s: %w", rel, err)
			}
		} else {
			absTarget, err := filepath.Abs(src)
			if err != nil {
				return warnings, fmt.Errorf("resolving absolute path for %s: %w", rel, err)
			}
			_ = os.Remove(dst)
			if err := os.Symlink(absTarget, dst); err != nil {
				return warnings, fmt.Errorf("symlinking %s: %w", rel, err)
			}
		}
	}

	return warnings, nil
}

// RunSyncPush pushes files from the repo root to a specific worktree.
func RunSyncPush(appCfg config.AppConfig, g git.Runner, dstWorktree string, copyOverride bool) ([]string, error) {
	repoRoot, inRepo, err := git.FindRepoRoot(g)
	if err != nil {
		return nil, err
	}
	if !inRepo {
		return nil, fmt.Errorf("not inside a git repository")
	}

	cfg, err := loadSyncConfigForPath(filepath.Join(repoRoot, ".git-ctx-sync.yaml"), appCfg.Worktree.DefaultMode)
	if err != nil {
		return nil, err
	}
	if len(cfg.Files) == 0 {
		return nil, nil
	}
	return SyncDirectionFiles(cfg, repoRoot, dstWorktree, Push, copyOverride)
}

// RunSyncPull pulls files from a non-primary worktree back to the repo root.
// It detects conflicts where the same file was modified in both worktrees.
func RunSyncPull(appCfg config.AppConfig, g git.Runner, srcWorktree string, copyOverride bool) ([]string, error) {
	repoRoot, inRepo, err := git.FindRepoRoot(g)
	if err != nil {
		return nil, err
	}
	if !inRepo {
		return nil, fmt.Errorf("not inside a git repository")
	}

	cfg, err := loadSyncConfigForPath(filepath.Join(repoRoot, ".git-ctx-sync.yaml"), appCfg.Worktree.DefaultMode)
	if err != nil {
		return nil, err
	}
	if len(cfg.Files) == 0 {
		return nil, nil
	}
	return SyncDirectionFiles(cfg, srcWorktree, repoRoot, Pull, copyOverride)
}

// loadSyncConfigForPath is an internal version of LoadSyncConfig that returns an
// error (rather than defaults) when the config file is missing.
func loadSyncConfigForPath(cfgPath string, defaultMode string) (SyncConfig, error) {
	cfg := SyncConfig{Mode: defaultMode}

	data, err := os.ReadFile(cfgPath)
	if os.IsNotExist(err) {
		return cfg, fmt.Errorf("no .git-ctx-sync.yaml found in %s", filepath.Dir(cfgPath))
	}
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", cfgPath, err)
	}
	if cfg.Mode == "" {
		cfg.Mode = defaultMode
	}
	return cfg, nil
}

// WatchConfig holds configuration for a watch loop.
type WatchConfig struct {
	// Debounce is the minimum interval between sync events for the same file.
	Debounce time.Duration
}

// DefaultWatchConfig is the default WatchConfig.
var DefaultWatchConfig = WatchConfig{
	Debounce: 500 * time.Millisecond,
}

// WatchLoop watches srcRoot for changes to the files listed in cfg and syncs
// them to dstRoot. It returns when the watcher is closed or on error.
func WatchLoop(cfg SyncConfig, srcRoot, dstRoot string, copyOverride bool, wcfg WatchConfig, onSync func([]string), onError func(error)) {
	debounce := wcfg.Debounce
	if debounce == 0 {
		debounce = DefaultWatchConfig.Debounce
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		onError(fmt.Errorf("creating watcher: %w", err))
		return
	}
	defer watcher.Close()

	// Collect watched dirs.
	watched := make(map[string]bool)
	for _, rel := range cfg.Files {
		dir := filepath.Dir(filepath.Join(srcRoot, rel))
		if !watched[dir] {
			if err := watcher.Add(dir); err != nil {
				onError(fmt.Errorf("watching %s: %w", dir, err))
				return
			}
			watched[dir] = true
		}
	}

	// Deduplicate file events per debounce window.
	type pendingSync struct {
		mu    sync.Mutex
		timer *time.Timer
		files []string
	}
	ps := &pendingSync{}

	scheduleSync := func() {
		ps.mu.Lock()
		if ps.timer != nil {
			ps.mu.Unlock()
			return
		}
		ps.mu.Unlock()
		ps.mu.Lock()
		ps.timer = time.AfterFunc(debounce, func() {
			ps.mu.Lock()
			files := ps.files
			ps.files = nil
			ps.timer = nil
			ps.mu.Unlock()
			warnings, err := SyncDirectionFiles(cfg, srcRoot, dstRoot, Push, copyOverride)
			if err != nil {
				onError(err)
				return
			}
			onSync(warnings)
			_ = files
		})
		ps.mu.Unlock()
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == 0 {
				continue
			}
			for _, rel := range cfg.Files {
				if filepath.Join(srcRoot, rel) == event.Name {
					ps.mu.Lock()
					ps.files = append(ps.files, rel)
					ps.mu.Unlock()
					scheduleSync()
					break
				}
			}
		case err := <-watcher.Errors:
			if err != nil {
				onError(fmt.Errorf("watcher error: %w", err))
				return
			}
		}
	}
}

