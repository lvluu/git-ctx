package main

import (
	"bufio"
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
)

type AutoResolution struct {
	ProfileKey string
	ScopeFlag  string // "--local" or "--global"
	WorkDir    string // repo root for --local, empty for --global
	RCPath     string
}

type AutoResolver struct {
	GetRepoRoot    func() (root string, inRepo bool, err error)
	GetHomeDir     func() (string, error)
	ReadFile       func(path string) ([]byte, error)
	FileExists     func(path string) bool
	DirectoryRules []DirectoryRule
	GetCurrentDir  func() (string, error)
}

func (r AutoResolver) Resolve() (AutoResolution, error) {
	repoRoot, inRepo, err := r.GetRepoRoot()
	if err != nil {
		return AutoResolution{}, err
	}

	if inRepo {
		projectRC := filepath.Join(repoRoot, ".gitprofilerc")
		if r.FileExists(projectRC) {
			key, err := r.readProfileKey(projectRC)
			if err != nil {
				return AutoResolution{}, err
			}
			return AutoResolution{ProfileKey: key, ScopeFlag: "--local", WorkDir: repoRoot, RCPath: projectRC}, nil
		}
	}

	homeDir, err := r.GetHomeDir()
	if err != nil {
		return AutoResolution{}, err
	}
	homeRC := filepath.Join(homeDir, ".gitprofilerc")
	if r.FileExists(homeRC) {
		key, err := r.readProfileKey(homeRC)
		if err != nil {
			return AutoResolution{}, err
		}
		return AutoResolution{ProfileKey: key, ScopeFlag: "--global", WorkDir: "", RCPath: homeRC}, nil
	}

	// Fallback: check directory rules from global config.
	if len(r.DirectoryRules) > 0 && r.GetCurrentDir != nil {
		currentDir, err := r.GetCurrentDir()
		if err == nil {
			cfg := AppConfig{DirectoryRules: r.DirectoryRules}
			if profile, ok := cfg.MatchDirectoryRule(currentDir); ok {
				scopeFlag := "--global"
				workDir := ""
				if inRepo {
					scopeFlag = "--local"
					workDir = repoRoot
				}
				return AutoResolution{
					ProfileKey: profile,
					ScopeFlag:  scopeFlag,
					WorkDir:    workDir,
					RCPath:     "(directory rule)",
				}, nil
			}
		}
	}

	return AutoResolution{}, fmt.Errorf("no .gitprofilerc found and no directory rule matched")
}

func (r AutoResolver) readProfileKey(path string) (string, error) {
	b, err := r.ReadFile(path)
	if err != nil {
		return "", err
	}
	key, err := parseGitProfileRC(b)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}

func parseGitProfileRC(content []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "profile=") {
			value := strings.TrimSpace(line[len("profile="):])
			if value == "" {
				return "", fmt.Errorf("profile is empty")
			}
			return value, nil
		}
		if strings.HasPrefix(lower, "profile:") {
			value := strings.TrimSpace(line[len("profile:"):])
			if value == "" {
				return "", fmt.Errorf("profile is empty")
			}
			return value, nil
		}

		return line, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no profile specified")
}
