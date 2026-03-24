package worktree

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// HookRunner executes hook commands.
type HookRunner interface {
	Run(hook, worktreePath string, env map[string]string) error
}

// ExecHookRunner runs hooks via exec.Command.
type ExecHookRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Run executes a hook command in the worktree directory.
func (r *ExecHookRunner) Run(hook, worktreePath string, env map[string]string) error {
	cmd := exec.Command("/bin/sh", "-c", hook)
	cmd.Dir = worktreePath
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook failed: %s: %w", hook, err)
	}
	return nil
}

// RunHooks executes a list of hooks sequentially. Stops on first failure.
func RunHooks(runner HookRunner, hooks []string, worktreePath, branch, repoRoot string) error {
	env := map[string]string{
		"GIT_CTX_WORKTREE_PATH":   worktreePath,
		"GIT_CTX_WORKTREE_BRANCH": branch,
		"GIT_CTX_REPO_ROOT":       repoRoot,
	}

	for _, hook := range hooks {
		if err := runner.Run(hook, worktreePath, env); err != nil {
			return err
		}
	}
	return nil
}
