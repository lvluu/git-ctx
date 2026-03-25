// Package worktree handles git worktree operations.
package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
