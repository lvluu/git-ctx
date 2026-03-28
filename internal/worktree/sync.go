// Package worktree handles git worktree operations.
package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

// PushFiles syncs configured files from the primary repo to a worktree (primary → worktree).
// Always uses copy mode since the destination is an independent worktree.
func PushFiles(appCfg config.AppConfig, g git.Runner, worktreePath string) ([]string, error) {
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

	return SyncFiles(cfg, repoRoot, worktreePath, true)
}

// PullFiles syncs configured files from a worktree back to the primary repo (worktree → primary).
// Always uses copy mode.
func PullFiles(appCfg config.AppConfig, g git.Runner, worktreePath string) ([]string, error) {
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

	return SyncFiles(cfg, worktreePath, repoRoot, true)
}

// WatchFiles watches a worktree for changes to synced files and automatically
// re-syncs them back to the primary repo (worktree → primary) on every write.
// It blocks until the watcher is closed (Ctrl+C).
func WatchFiles(appCfg config.AppConfig, g git.Runner, worktreePath string) error {
	repoRoot, inRepo, err := git.FindRepoRoot(g)
	if err != nil {
		return err
	}
	if !inRepo {
		return fmt.Errorf("not inside a git repository")
	}

	syncCfgPath := filepath.Join(repoRoot, ".git-ctx-sync.yaml")
	cfg, err := LoadSyncConfig(syncCfgPath, appCfg.Worktree.DefaultMode)
	if err != nil {
		return err
	}

	if len(cfg.Files) == 0 {
		return fmt.Errorf("no files configured in .git-ctx-sync.yaml")
	}

	// Build the set of directories to watch (deduplicated).
	watchDirs := make(map[string]bool)
	for _, rel := range cfg.Files {
		abs := filepath.Join(worktreePath, rel)
		watchDirs[filepath.Dir(abs)] = true
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	for dir := range watchDirs {
		if err := watcher.Add(dir); err != nil {
			return fmt.Errorf("watching directory %s: %w", dir, err)
		}
	}

	fmt.Printf("Watching %d path(s) for changes (Ctrl+C to stop)...\n", len(watchDirs))

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Only react to writes and creates.
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			rel, err := filepath.Rel(worktreePath, event.Name)
			if err != nil {
				continue
			}
			inList := false
			for _, f := range cfg.Files {
				if f == rel {
					inList = true
					break
				}
			}
			if !inList {
				continue
			}
			fmt.Printf("Change detected: %s — syncing to primary repo...\n", rel)
			_, err = PullFiles(appCfg, g, worktreePath)
			if err != nil {
				fmt.Printf("Sync error: %v\n", err)
			} else {
				fmt.Printf("Synced: %s\n", rel)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Printf("Watcher error: %v\n", err)
		}
	}
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
