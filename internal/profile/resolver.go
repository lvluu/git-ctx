package profile

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
}

// DirectoryRule maps a directory path prefix to a profile name.
type DirectoryRule struct {
	Pattern string `yaml:"pattern"`
	Profile string `yaml:"profile"`
}

// Resolve determines which profile to apply and where.
func (r Resolver) Resolve() (ResolutionResult, error) {
	repoRoot, inRepo, err := r.GetRepoRoot()
	if err != nil {
		return ResolutionResult{}, err
	}

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

	homeRc := r.getHomeRCPath()
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

	if len(r.DirectoryRules) > 0 {
		currentDir, err := r.GetCurrentDir()
		if err != nil {
			return ResolutionResult{}, err
		}
		profile, ok := r.matchDirectoryRule(currentDir)
		if ok {
			scopeFlag := "--global"
			if inRepo {
				scopeFlag = "--local"
			}
			return ResolutionResult{
				ProfileKey: profile,
				RCPath:     "directory rule",
				ScopeFlag:  scopeFlag,
				WorkDir:    repoRoot,
			}, nil
		}
	}

	return ResolutionResult{}, nil
}

func (r Resolver) getHomeRCPath() string {
	homeDir, _ := r.GetHomeDir()
	return homeDir + "/.gitprofilerc"
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
	if pattern == "~" || pattern == "~/" {
		return true
	}
	// Simple prefix matching for now
	return len(dir) >= len(pattern) && dir[:len(pattern)] == pattern
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
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
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
