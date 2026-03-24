package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHookRunner struct {
	calls []struct {
		hook, dir string
		env       map[string]string
	}
	err     error
	errHook string
}

func (f *fakeHookRunner) Run(hook, worktreePath string, env map[string]string) error {
	f.calls = append(f.calls, struct {
		hook, dir string
		env       map[string]string
	}{hook, worktreePath, env})
	if f.err != nil && (f.errHook == "" || f.errHook == hook) {
		return f.err
	}
	return nil
}

func TestRunHooks_EmptyList(t *testing.T) {
	runner := &fakeHookRunner{}
	err := runHooks(runner, []string{}, "/path/to/worktree", "my-branch", "/path/to/repo")
	assert.NoError(t, err)
	assert.Len(t, runner.calls, 0)
}

func TestRunHooks_SingleHook(t *testing.T) {
	runner := &fakeHookRunner{}
	hooks := []string{"bun install"}

	err := runHooks(runner, hooks, "/worktree", "feature-branch", "/repo")
	require.NoError(t, err)
	require.Len(t, runner.calls, 1)

	assert.Equal(t, "bun install", runner.calls[0].hook)
	assert.Equal(t, "/worktree", runner.calls[0].dir)
	assert.Equal(t, "feature-branch", runner.calls[0].env["GIT_CTX_WORKTREE_BRANCH"])
	assert.Equal(t, "/worktree", runner.calls[0].env["GIT_CTX_WORKTREE_PATH"])
	assert.Equal(t, "/repo", runner.calls[0].env["GIT_CTX_REPO_ROOT"])
}

func TestRunHooks_MultipleHooks(t *testing.T) {
	runner := &fakeHookRunner{}
	hooks := []string{"bun install", "npm run setup", "echo 'done'"}

	err := runHooks(runner, hooks, "/worktree", "branch", "/repo")
	require.NoError(t, err)
	require.Len(t, runner.calls, 3)

	assert.Equal(t, "bun install", runner.calls[0].hook)
	assert.Equal(t, "npm run setup", runner.calls[1].hook)
	assert.Equal(t, "echo 'done'", runner.calls[2].hook)
}

func TestRunHooks_StopsOnFirstError(t *testing.T) {
	runner := &fakeHookRunner{
		err:     errors.New("hook failed"),
		errHook: "npm install",
	}
	hooks := []string{"bun install", "npm install", "echo 'done'"}

	err := runHooks(runner, hooks, "/worktree", "branch", "/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed")
	assert.Len(t, runner.calls, 2)
}

func TestRunHooks_EnvironmentVariables(t *testing.T) {
	runner := &fakeHookRunner{}
	hooks := []string{"echo test"}

	err := runHooks(runner, hooks, "/path/to/my-feature", "my-feature", "/path/to/repo")
	require.NoError(t, err)

	assert.Equal(t, "my-feature", runner.calls[0].env["GIT_CTX_WORKTREE_BRANCH"])
	assert.Equal(t, "/path/to/my-feature", runner.calls[0].env["GIT_CTX_WORKTREE_PATH"])
	assert.Equal(t, "/path/to/repo", runner.calls[0].env["GIT_CTX_REPO_ROOT"])
}

func TestExecHookRunner_CommandExecution(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &ExecHookRunner{Stdout: &stdout, Stderr: &stderr}

	err := runner.Run("echo 'hello world'", "/tmp", map[string]string{"MY_VAR": "value"})
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "hello world")
}

func TestExecHookRunner_CommandFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &ExecHookRunner{Stdout: &stdout, Stderr: &stderr}

	err := runner.Run("exit 1", "/tmp", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook failed")
}

func TestExecHookRunner_NonexistentCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	runner := &ExecHookRunner{Stdout: &stdout, Stderr: &stderr}

	err := runner.Run("nonexistent_command_xyz", "/tmp", nil)
	require.Error(t, err)
}
