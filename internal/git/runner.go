// Package git provides git operations and abstractions.
package git

import (
	"errors"
	"os/exec"
)

// Sentinel errors for common git failure modes.
var (
	ErrNotGitRepo           = errors.New("not a git repository")
	ErrGitConfigKeyNotFound = errors.New("git config key not found")
)

// Runner defines the interface for executing git commands.
type Runner interface {
	// Output executes a git command and returns its stdout.
	Output(dir string, args ...string) ([]byte, error)
	// Run executes a git command and checks for errors.
	Run(dir string, args ...string) error
}

// ExecRunner implements Runner using actual git CLI calls.
type ExecRunner struct{}

// Output executes a git command and returns its stdout.
func (g ExecRunner) Output(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}

	// Normalize common git failure modes into sentinel errors.
	if code, ok := exitCode(err); ok {
		// `git rev-parse --show-toplevel` returns 128 when not in a repo.
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" && code != 0 {
			return nil, ErrNotGitRepo
		}
		// `git config --<scope> --get <key>` returns 1 when the key is unset.
		for i := 0; i < len(args); i++ {
			if args[i] == "--get" && code == 1 {
				return nil, ErrGitConfigKeyNotFound
			}
		}
	}

	return nil, err
}

// Run executes a git command and checks for errors.
func (g ExecRunner) Run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}

func exitCode(err error) (int, bool) {
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ProcessState != nil {
		return ee.ProcessState.ExitCode(), true
	}
	return 0, false
}
