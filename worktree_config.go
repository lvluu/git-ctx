package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SyncConfig is the per-repo worktree sync configuration from .git-ctx-sync.yaml.
type SyncConfig struct {
	// Mode is "symlink" (default) or "copy".
	Mode string `yaml:"mode"`
	// Files lists paths relative to the repo root to sync into each worktree.
	Files []string `yaml:"files"`
}

// loadSyncConfig reads .git-ctx-sync.yaml from cfgPath.
// If the file does not exist, an empty config with defaultMode is returned.
// defaultMode comes from AppConfig.Worktree.DefaultMode.
func loadSyncConfig(cfgPath string, defaultMode string) (SyncConfig, error) {
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

// syncFiles creates symlinks or copies for each file in cfg.Files from srcRoot to dstRoot.
// copyOverride=true forces copy regardless of cfg.Mode (for the --copy flag).
// Returns a list of warning strings for files that don't exist in srcRoot (not fatal).
func syncFiles(cfg SyncConfig, srcRoot, dstRoot string, copyOverride bool) (warnings []string, err error) {
	useCopy := copyOverride || cfg.Mode == "copy"

	for _, rel := range cfg.Files {
		src := filepath.Join(srcRoot, rel)
		dst := filepath.Join(dstRoot, rel)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("skipping %s: not found in source worktree", rel))
			continue
		}

		// Ensure parent directory exists in destination.
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

	_, err = io.Copy(out, in)
	return err
}
