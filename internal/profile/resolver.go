package profile

import (
	"path/filepath"
	"strings"
)

// RuleType defines the matching strategy for a directory rule.
type RuleType string

const (
	RuleTypePath   RuleType = "path"
	RuleTypeRemote RuleType = "remote"
	RuleTypeEmail  RuleType = "email"
)

// Confidence represents how strongly a matcher believes its result.
type Confidence int

const (
	ConfidenceNone  Confidence = 0
	ConfidenceLow   Confidence = 25  // closest-path fallback
	ConfidenceMedium Confidence = 50  // email or remote match
	ConfidenceHigh  Confidence = 100 // exact rule match (future)
)

// ResolutionResult contains the results of resolving which profile to apply.
type ResolutionResult struct {
	ProfileKey string // The profile key to apply
	RCPath     string // Path to the .gitprofilerc file (if resolved from file)
	ScopeFlag  string // --local or --global
	WorkDir    string // Working directory for git commands
}

// Resolver resolves which profile to apply based on .gitprofilerc or directory rules.
type Resolver struct {
	GetRepoRoot    func() (string, bool, error)
	GetHomeDir     func() (string, error)
	ReadFile       func(string) ([]byte, error)
	FileExists     func(string) bool
	DirectoryRules []DirectoryRule
	GetCurrentDir  func() (string, error)
	// GitRunner is optional; when set, remote and email matchers can probe the live repo.
	GitRunner       GitRunnerForResolver
	GitConfigGetter func(repoRoot, key string) (string, error)
}

// GitRunnerForResolver is the subset of git.Runner needed by the resolver.
type GitRunnerForResolver interface {
	Output(dir string, args ...string) ([]byte, error)
}

// DirectoryRule maps a directory path prefix to a profile name.
type DirectoryRule struct {
	Pattern string   `yaml:"pattern"`
	Remote  string   `yaml:"remote"` // host/org substring to match in git remote URL
	Email   string   `yaml:"email"` // exact email to match against git user.email
	Type    RuleType `yaml:"type"`
	Profile string   `yaml:"profile"`
}

// MatchResult records a candidate match with its confidence score.
type MatchResult struct {
	Profile   string
	Confidence Confidence
	RuleType  RuleType
}

// Resolve determines which profile to apply and where.
// Priority: .gitprofilerc > ~/.gitprofilerc > [remote match > email match > closest-path].
func (r Resolver) Resolve() (ResolutionResult, error) {
	repoRoot, inRepo, err := r.GetRepoRoot()
	if err != nil {
		return ResolutionResult{}, err
	}

	// 1. Check repo-local .gitprofilerc
	if inRepo {
		rcPath := repoRoot + "/.gitprofilerc"
		if r.FileExists(rcPath) {
			data, err := r.ReadFile(rcPath)
			if err != nil {
				return ResolutionResult{}, err
			}
			profileKey := ParseGitProfileRC(string(data))
			if profileKey != "" {
				return ResolutionResult{
					ProfileKey: profileKey,
					RCPath:     rcPath,
					ScopeFlag:  "--local",
					WorkDir:    repoRoot,
				}, nil
			}
		}
	}

	// 2. Check home .gitprofilerc
	homeRc, err := r.getHomeRCPath()
	if err != nil {
		return ResolutionResult{}, err
	}
	if r.FileExists(homeRc) {
		data, err := r.ReadFile(homeRc)
		if err != nil {
			return ResolutionResult{}, err
		}
		profileKey := ParseGitProfileRC(string(data))
		if profileKey != "" {
			return ResolutionResult{
				ProfileKey: profileKey,
				RCPath:     homeRc,
				ScopeFlag:  "--global",
				WorkDir:    "",
			}, nil
		}
	}

	// 3. Smart directory rules
	if len(r.DirectoryRules) > 0 {
		currentDir, err := r.GetCurrentDir()
		if err != nil {
			return ResolutionResult{}, err
		}

		result := r.resolveBySmartRules(currentDir, repoRoot, inRepo)
		if result.ProfileKey != "" {
			return result, nil
		}
	}

	return ResolutionResult{}, nil
}

// resolveBySmartRules tries all directory rules using remote > email > closest-path priority.
func (r Resolver) resolveBySmartRules(currentDir, repoRoot string, inRepo bool) ResolutionResult {
	var candidates []MatchResult

	// Try remote and email matchers (only when inside a repo with a runner available)
	if inRepo && r.GitRunner != nil {
		// Remote matcher: highest priority
		if profile := r.matchRemote(repoRoot); profile != "" {
			candidates = append(candidates, MatchResult{
				Profile:   profile,
				Confidence: ConfidenceMedium,
				RuleType:  RuleTypeRemote,
			})
		}

		// Email matcher
		if profile := r.matchEmail(repoRoot); profile != "" {
			candidates = append(candidates, MatchResult{
				Profile:   profile,
				Confidence: ConfidenceMedium,
				RuleType:  RuleTypeEmail,
			})
		}
	}

	// Closest-path fallback: use the rule with the longest pattern prefix match.
	if profile := r.matchClosestPath(currentDir); profile != "" {
		candidates = append(candidates, MatchResult{
			Profile:   profile,
			Confidence: ConfidenceLow,
			RuleType:  RuleTypePath,
		})
	}

	// Pick the highest-confidence candidate.
	best := MatchResult{}
	for _, c := range candidates {
		if c.Confidence > best.Confidence {
			best = c
		}
	}

	if best.Profile == "" {
		return ResolutionResult{}
	}

	scopeFlag := "--global"
	if inRepo {
		scopeFlag = "--local"
	}
	return ResolutionResult{
		ProfileKey: best.Profile,
		RCPath:     "directory rule",
		ScopeFlag:  scopeFlag,
		WorkDir:    repoRoot,
	}
}

// matchRemote returns the profile key if any remote URL contains r.Remote as a substring.
// repoRoot is passed directly so we don't need GetRepoRoot again.
func (r Resolver) matchRemote(repoRoot string) string {
	if r.GitRunner == nil {
		return ""
	}
	for _, rule := range r.DirectoryRules {
		if rule.Type == RuleTypeRemote && rule.Remote != "" {
			// Fetch the remote URL to check against.
			remoteOut, err := r.GitRunner.Output(repoRoot, "remote", "get-url", "origin")
			if err != nil {
				continue
			}
			// get-url may return the URL with a newline.
			remoteURL := trimSpace(string(remoteOut))
			if strings.Contains(remoteURL, rule.Remote) {
				return rule.Profile
			}
		}
	}
	return ""
}

// matchEmail returns the profile key if the repo's git user.email matches any email rule.
// repoRoot is passed directly so we don't need GetRepoRoot again.
func (r Resolver) matchEmail(repoRoot string) string {
	if r.GitRunner == nil {
		return ""
	}
	for _, rule := range r.DirectoryRules {
		if rule.Type == RuleTypeEmail && rule.Email != "" {
			emailOut, err := r.GitRunner.Output(repoRoot, "config", "user.email")
			if err != nil {
				continue
			}
			email := trimSpace(string(emailOut))
			if email == rule.Email {
				return rule.Profile
			}
		}
	}
	return ""
}

// matchClosestPath returns the profile for the longest matching path prefix.
// This is the legacy directory-rule behaviour preserved as the final fallback.
func (r Resolver) matchClosestPath(dir string) string {
	best := ""
	bestLen := -1
	for _, rule := range r.DirectoryRules {
		// Only apply path rules: explicit RuleTypePath or legacy pattern-with-no-type.
		if rule.Type == RuleTypeRemote || rule.Type == RuleTypeEmail {
			continue
		}
		pattern := rule.Pattern
		if pattern == "" {
			continue
		}

		// Expand ~/ to home dir.
		if strings.HasPrefix(pattern, "~/") {
			homeDir, err := r.GetHomeDir()
			if err == nil {
				pattern = filepath.Join(homeDir, pattern[2:])
			}
		} else if pattern == "~" {
			homeDir, err := r.GetHomeDir()
			if err == nil {
				pattern = homeDir
			}
		}

		// Segment-aligned prefix match.
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
	return best
}

func (r Resolver) getHomeRCPath() (string, error) {
	homeDir, err := r.GetHomeDir()
	if err != nil {
		return "", err
	}
	return homeDir + "/.gitprofilerc", nil
}

func (r Resolver) matchDirectoryRule(dir string) (string, bool) {
	for _, rule := range r.DirectoryRules {
		if matches(rule.Pattern, dir) {
			return rule.Profile, true
		}
	}
	return "", false
}

func matches(pattern, dir string) bool {
	// Match home directory exactly
	if pattern == "~" {
		return dir == "~" || dir == ""
	}
	if pattern == "~/" {
		return len(dir) >= 2 && dir[:2] == "~/"
	}
	// Require path segment match: /work must match /work or /work/ but NOT /workspace
	if dir == pattern {
		return true
	}
	return len(dir) > len(pattern) && dir[:len(pattern)] == pattern && dir[len(pattern)] == '/'
}

// ParseGitProfileRC parses a .gitprofilerc file content and returns the profile key.
func ParseGitProfileRC(content string) string {
	for _, line := range splitLines(content) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if key, ok := parseProfileLine(line); ok {
			return key
		}
	}
	return ""
}

func splitLines(s string) []string {
	result := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r' || s[start] == '\n') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r' || s[end-1] == '\n') {
		end--
	}
	return s[start:end]
}

func parseProfileLine(line string) (string, bool) {
	if len(line) >= 8 && line[:8] == "profile=" {
		return trimSpace(line[8:]), true
	}
	if len(line) >= 8 && line[:8] == "profile:" {
		return trimSpace(line[8:]), true
	}
	if len(line) >= 8 && line[:7] == "profile" && (line[7] == '=' || line[7] == ':') {
		return trimSpace(line[8:]), true
	}
	// Plain profile name
	for _, c := range line {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return "", false
		}
	}
	return line, true
}
