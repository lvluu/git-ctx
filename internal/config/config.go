// Package config handles application configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppConfig is the unified configuration for git-ctx stored at ~/.git-ctx.yaml.
type AppConfig struct {
	ProfilesPath   string          `yaml:"profiles_path"`
	DirectoryRules []DirectoryRule `yaml:"directory_rules"`
	Worktree       WorktreeConfig  `yaml:"worktree"`
	path           string
}

// DirectoryRule maps a directory path prefix to a profile name.
type DirectoryRule struct {
	Pattern string `yaml:"pattern"`
	Profile string `yaml:"profile"`
}

// WorktreeHooks holds hook commands that run at various lifecycle points.
type WorktreeHooks struct {
	PostCreate []string `yaml:"post_create"`
}

// WorktreeConfig holds worktree-related settings.
type WorktreeConfig struct {
	DefaultMode string        `yaml:"default_mode"`
	Hooks       WorktreeHooks `yaml:"hooks"`
}

// MatchDirectoryRule finds the best-matching profile for dir using longest-prefix matching.
func (cfg AppConfig) MatchDirectoryRule(dir string) (profile string, ok bool) {
	homeDir, _ := os.UserHomeDir()
	best := ""
	bestLen := -1

	for _, rule := range cfg.DirectoryRules {
		pattern := rule.Pattern
		if strings.HasPrefix(pattern, "~/") {
			pattern = filepath.Join(homeDir, pattern[2:])
		} else if pattern == "~" {
			pattern = homeDir
		}

		if !strings.HasSuffix(pattern, string(filepath.Separator)) {
			pattern += string(filepath.Separator)
		}
		checkDir := dir
		if !strings.HasSuffix(checkDir, string(filepath.Separator)) {
			checkDir += string(filepath.Separator)
		}

		if strings.HasPrefix(checkDir, pattern) && len(pattern) > bestLen {
			best = rule.Profile
			bestLen = len(pattern)
		}
	}

	if bestLen >= 0 {
		return best, true
	}
	return "", false
}

// DefaultPath returns the default config path (~/.git-ctx.yaml).
func DefaultPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".git-ctx.yaml"
	}
	return filepath.Join(homeDir, ".git-ctx.yaml")
}

// Load reads the YAML config file at cfgPath.
// If the file does not exist, defaults are returned without error.
func Load(cfgPath string) (AppConfig, error) {
	cfg := defaults(cfgPath)

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
	cfg.path = cfgPath

	if cfg.Worktree.DefaultMode == "" {
		cfg.Worktree.DefaultMode = "symlink"
	}
	if cfg.ProfilesPath == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.ProfilesPath = filepath.Join(homeDir, ".git-ctx-profiles.json")
	}

	return cfg, nil
}

// Init writes a scaffolded config file to cfgPath.
func Init(cfgPath string, force bool) error {
	if _, err := os.Stat(cfgPath); err == nil && !force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", cfgPath)
	}

	homeDir, _ := os.UserHomeDir()
	scaffold := fmt.Sprintf(`# git-ctx configuration
profiles_path: %s

# Map directory prefixes to profiles.
# Longest matching prefix wins.
directory_rules:
  - pattern: "~/work"
    profile: work
  - pattern: "~/personal"
    profile: personal

worktree:
  default_mode: symlink  # symlink | copy
`, filepath.Join(homeDir, ".git-ctx-profiles.json"))

	return os.WriteFile(cfgPath, []byte(scaffold), 0644)
}

func defaults(cfgPath string) AppConfig {
	homeDir, _ := os.UserHomeDir()
	return AppConfig{
		ProfilesPath: filepath.Join(homeDir, ".git-ctx-profiles.json"),
		Worktree: WorktreeConfig{
			DefaultMode: "symlink",
		},
		path: cfgPath,
	}
}
