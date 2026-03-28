package profile

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeGitRunner captures and returns configured outputs for git commands.
type fakeGitRunner struct {
	outputs map[string][]byte
	errs    map[string]error
}

func newFakeRunner() *fakeGitRunner {
	return &fakeGitRunner{
		outputs: make(map[string][]byte),
		errs:    make(map[string]error),
	}
}

func (f *fakeGitRunner) Output(dir string, args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	return nil, errors.New("unconfigured git output for: " + key)
}

func (f *fakeGitRunner) setOutput(args []string, out string) {
	f.outputs[strings.Join(args, " ")] = []byte(out)
}

func (f *fakeGitRunner) setError(args []string, err error) {
	f.errs[strings.Join(args, " ")] = err
}

// Stub implementations so we never hit real filesystem.
func stubGetRepoRoot() (string, bool, error) { return "", false, nil }
func stubGetHomeDir() (string, error)         { return "/home/user", nil }
func stubReadFile(string) ([]byte, error)     { return nil, os.ErrNotExist }
func stubFileExists(string) bool               { return false }
func stubGetCurrentDir() (string, error)       { return "/home/user/work/project", nil }

// --- Remote matcher tests ---

func TestResolve_RemoteMatcher(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"remote", "get-url", "origin"}, "https://github.com/your-org/repo.git")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  stubGetCurrentDir,
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeRemote, Remote: "github.com/your-org", Profile: "work"},
			{Type: RuleTypeRemote, Remote: "github.com/other-org", Profile: "other"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "work" {
		t.Errorf("expected 'work', got %q", res.ProfileKey)
	}
	if res.ScopeFlag != "--local" {
		t.Errorf("expected --local, got %q", res.ScopeFlag)
	}
}

func TestResolve_RemoteMatcherNoMatch(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"remote", "get-url", "origin"}, "https://github.com/another-org/repo.git")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  stubGetCurrentDir,
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeRemote, Remote: "github.com/your-org", Profile: "work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "" {
		t.Errorf("expected no match, got %q", res.ProfileKey)
	}
}

func TestResolve_RemoteMatcherNoOrigin(t *testing.T) {
	runner := newFakeRunner()
	runner.setError([]string{"remote", "get-url", "origin"}, errors.New("no remote"))

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  stubGetCurrentDir,
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeRemote, Remote: "github.com/your-org", Profile: "work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "" {
		t.Errorf("expected no match, got %q", res.ProfileKey)
	}
}

// --- Email matcher tests ---

func TestResolve_EmailMatcher(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"config", "user.email"}, "levi@company.com\n")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  stubGetCurrentDir,
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeEmail, Email: "levi@company.com", Profile: "work"},
			{Type: RuleTypeEmail, Email: "other@example.com", Profile: "other"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "work" {
		t.Errorf("expected 'work', got %q", res.ProfileKey)
	}
}

func TestResolve_EmailMatcherNoMatch(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"config", "user.email"}, "stranger@unknown.com\n")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  stubGetCurrentDir,
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeEmail, Email: "levi@company.com", Profile: "work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "" {
		t.Errorf("expected no match, got %q", res.ProfileKey)
	}
}

// --- Closest-path fallback tests ---

func TestResolve_ClosestPathFallback(t *testing.T) {
	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  func() (string, error) { return "/home/user/work/project", nil },
		GitRunner:      nil, // no runner → no remote/email
		DirectoryRules: []DirectoryRule{
			{Pattern: "/home/user", Profile: "root"},
			{Pattern: "/home/user/work", Profile: "work"},
			{Pattern: "/home/user/work/project", Profile: "project"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "project" {
		t.Errorf("expected 'project', got %q", res.ProfileKey)
	}
}

func TestResolve_ClosestPathWithHome(t *testing.T) {
	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/personal/repo", true, nil },
		GetHomeDir:     func() (string, error) { return "/home/user", nil },
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  func() (string, error) { return "/home/user/personal/repo", nil },
		GitRunner:      nil,
		DirectoryRules: []DirectoryRule{
			{Pattern: "~/personal", Profile: "personal"},
			{Pattern: "~/", Profile: "default"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "personal" {
		t.Errorf("expected 'personal', got %q", res.ProfileKey)
	}
}

// --- Priority: remote > email > closest-path ---

func TestResolve_PriorityRemoteWinsOverEmail(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"remote", "get-url", "origin"}, "https://github.com/your-org/repo.git")
	runner.setOutput([]string{"config", "user.email"}, "levi@company.com\n")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  stubGetCurrentDir,
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeRemote, Remote: "github.com/your-org", Profile: "remote-work"},
			{Type: RuleTypeEmail, Email: "levi@company.com", Profile: "email-work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "remote-work" {
		t.Errorf("expected remote match to win, got %q", res.ProfileKey)
	}
}

func TestResolve_PriorityEmailWinsOverClosestPath(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"config", "user.email"}, "levi@company.com\n")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  func() (string, error) { return "/home/user/work/repo", nil },
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeEmail, Email: "levi@company.com", Profile: "email-work"},
			{Pattern: "/home/user/work", Profile: "path-work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "email-work" {
		t.Errorf("expected email match to win over path, got %q", res.ProfileKey)
	}
}

func TestResolve_PriorityRemoteWinsOverClosestPath(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"remote", "get-url", "origin"}, "https://github.com/your-org/repo.git")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  func() (string, error) { return "/home/user/work/repo", nil },
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeRemote, Remote: "github.com/your-org", Profile: "remote-work"},
			{Pattern: "/home/user/work", Profile: "path-work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "remote-work" {
		t.Errorf("expected remote match to win over path, got %q", res.ProfileKey)
	}
}

// --- gitprofilerc takes precedence over all directory rules ---

func TestResolve_GitprofilercTakesPrecedence(t *testing.T) {
	runner := newFakeRunner()
	runner.setOutput([]string{"config", "user.email"}, "levi@company.com\n")

	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile: func(path string) ([]byte, error) {
			if path == "/home/user/work/repo/.gitprofilerc" {
				return []byte("profile=local-work"), nil
			}
			return nil, os.ErrNotExist
		},
		FileExists: func(path string) bool {
			return path == "/home/user/work/repo/.gitprofilerc"
		},
		GetCurrentDir:  func() (string, error) { return "/home/user/work/repo", nil },
		GitRunner:      runner,
		DirectoryRules: []DirectoryRule{
			{Type: RuleTypeEmail, Email: "levi@company.com", Profile: "email-work"},
			{Pattern: "/home/user/work", Profile: "path-work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "local-work" {
		t.Errorf("expected gitprofilerc to win, got %q", res.ProfileKey)
	}
	if res.ScopeFlag != "--local" {
		t.Errorf("expected --local, got %q", res.ScopeFlag)
	}
}

// --- Legacy pattern-with-no-type treated as path ---

func TestResolve_LegacyPatternNoType(t *testing.T) {
	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "/home/user/work/repo", true, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  func() (string, error) { return "/home/user/work/repo", nil },
		GitRunner:      nil,
		DirectoryRules: []DirectoryRule{
			{Pattern: "/home/user/work", Profile: "work"}, // no Type → path
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ProfileKey != "work" {
		t.Errorf("expected legacy pattern to match as path, got %q", res.ProfileKey)
	}
}

// --- Scope flag: --global when not in a repo ---

func TestResolve_ScopeFlagGlobalOutsideRepo(t *testing.T) {
	r := Resolver{
		GetRepoRoot:    func() (string, bool, error) { return "", false, nil },
		GetHomeDir:     stubGetHomeDir,
		ReadFile:       stubReadFile,
		FileExists:     stubFileExists,
		GetCurrentDir:  func() (string, error) { return "/home/user/work/repo", nil },
		GitRunner:      nil,
		DirectoryRules: []DirectoryRule{
			{Pattern: "/home/user/work", Profile: "work"},
		},
	}

	res, err := r.Resolve()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ScopeFlag != "--global" {
		t.Errorf("expected --global outside repo, got %q", res.ScopeFlag)
	}
}
