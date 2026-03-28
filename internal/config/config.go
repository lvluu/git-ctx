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

// RuleType defines the matching strategy for a directory rule.
type RuleType string

const (
	RuleTypePath   RuleType = "path"    // directory prefix match (default/legacy)
	RuleTypeRemote RuleType = "remote"  // match by git remote URL
	RuleTypeEmail  RuleType = "email"   // match by git user.email
)

// DirectoryRule maps a directory path prefix to a profile name.
type DirectoryRule struct {
	Pattern string  `yaml:"pattern"` // used for type=path
	Remote  string  `yaml:"remote"`  // used for type=remote: host/org substring
	Email   string  `yaml:"email"`   // used for type=email: exact email match
	Type    RuleType `yaml:"type"`   // defaults to "path" if Pattern is set
	Profile string  `yaml:"profile"` // profile to activate
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

// RuleTypeFor returns the effective rule type, defaulting to RuleTypePath.
func (r DirectoryRule) EffectiveType() RuleType {
	if r.Type != "" {
		return r.Type
	}
	// Legacy: a non-empty Pattern without an explicit type implies path
	if r.Pattern != "" {
		return RuleTypePath
	}
	return RuleTypePath
}

// String implements fmt.Stringer for RuleType.
func (t RuleType) String() string {
	return string(t)
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

# Rule matching priority (highest to lowest):
#   1. remote  – git remote URL contains the substring
#   2. email   – repo's git user.email equals the value
#   3. path    – longest directory-prefix match (existing behaviour)
# A rule needs at least one of: pattern (type=path), remote, or email.
directory_rules:
  - type: path
    pattern: "~/work"
    profile: work
  - type: remote
    remote: github.com/your-org
    profile: work
  - type: email
    email: levi@company.com
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
