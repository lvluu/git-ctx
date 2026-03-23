package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGitRunnerWithWorktree struct {
	fakeGitRunner
	worktreeAddCalls [][]string
}

func (f *fakeGitRunnerWithWorktree) Run(dir string, args ...string) error {
	if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
		f.worktreeAddCalls = append(f.worktreeAddCalls, args)
		return nil
	}
	return f.fakeGitRunner.Run(dir, args...)
}

func TestWorktreeAdd_CreatesBranchFromPath(t *testing.T) {
	git := &fakeGitRunnerWithWorktree{
		fakeGitRunner: fakeGitRunner{
			values: map[string]string{
				"__repo_root__": "/tmp/repo",
			},
		},
	}

	appCfg := AppConfig{Worktree: AppWorktreeConfig{DefaultMode: "symlink"}}
	cmd := buildWorktreeCmd(appCfg, git)
	cmd.SetArgs([]string{"add", "/tmp/my-feature"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Len(t, git.worktreeAddCalls, 1)

	call := git.worktreeAddCalls[0]
	assert.Equal(t, []string{"worktree", "add", "-b", "my-feature", "/tmp/my-feature"}, call)
}

func TestWorktreeAdd_WithCommitIsh(t *testing.T) {
	git := &fakeGitRunnerWithWorktree{
		fakeGitRunner: fakeGitRunner{
			values: map[string]string{
				"__repo_root__": "/tmp/repo",
			},
		},
	}

	appCfg := AppConfig{Worktree: AppWorktreeConfig{DefaultMode: "symlink"}}
	cmd := buildWorktreeCmd(appCfg, git)
	cmd.SetArgs([]string{"add", "/tmp/my-feature", "main"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Len(t, git.worktreeAddCalls, 1)

	call := git.worktreeAddCalls[0]
	assert.Equal(t, []string{"worktree", "add", "-b", "my-feature", "/tmp/my-feature", "main"}, call)
}

func TestWorktreeAdd_StripsTrailingSlash(t *testing.T) {
	git := &fakeGitRunnerWithWorktree{
		fakeGitRunner: fakeGitRunner{
			values: map[string]string{
				"__repo_root__": "/tmp/repo",
			},
		},
	}

	appCfg := AppConfig{Worktree: AppWorktreeConfig{DefaultMode: "symlink"}}
	cmd := buildWorktreeCmd(appCfg, git)
	cmd.SetArgs([]string{"add", "/tmp/my-feature/"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Len(t, git.worktreeAddCalls, 1)

	assert.Equal(t, "my-feature", git.worktreeAddCalls[0][3])
}

func TestWorktreeAdd_WithRelativePath(t *testing.T) {
	git := &fakeGitRunnerWithWorktree{
		fakeGitRunner: fakeGitRunner{
			values: map[string]string{
				"__repo_root__": "/tmp/repo",
			},
		},
	}

	appCfg := AppConfig{Worktree: AppWorktreeConfig{DefaultMode: "symlink"}}
	cmd := buildWorktreeCmd(appCfg, git)
	cmd.SetArgs([]string{"add", "../my-worktree"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Len(t, git.worktreeAddCalls, 1)

	call := git.worktreeAddCalls[0]
	assert.Equal(t, "my-worktree", call[3])
	assert.Equal(t, "../my-worktree", call[4])
}

func TestWorktreeAdd_WithNestedPath(t *testing.T) {
	git := &fakeGitRunnerWithWorktree{
		fakeGitRunner: fakeGitRunner{
			values: map[string]string{
				"__repo_root__": "/tmp/repo",
			},
		},
	}

	appCfg := AppConfig{Worktree: AppWorktreeConfig{DefaultMode: "symlink"}}
	cmd := buildWorktreeCmd(appCfg, git)
	cmd.SetArgs([]string{"add", "/home/user/projects/feature-x"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Len(t, git.worktreeAddCalls, 1)

	assert.Equal(t, "feature-x", git.worktreeAddCalls[0][3])
}

func TestWorktreeAdd_WithCommitHash(t *testing.T) {
	git := &fakeGitRunnerWithWorktree{
		fakeGitRunner: fakeGitRunner{
			values: map[string]string{
				"__repo_root__": "/tmp/repo",
			},
		},
	}

	appCfg := AppConfig{Worktree: AppWorktreeConfig{DefaultMode: "symlink"}}
	cmd := buildWorktreeCmd(appCfg, git)
	cmd.SetArgs([]string{"add", "/tmp/fix-bug", "abc123def456"})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Len(t, git.worktreeAddCalls, 1)

	call := git.worktreeAddCalls[0]
	assert.Equal(t, "-b", call[2])
	assert.Equal(t, "fix-bug", call[3])
	assert.Equal(t, "/tmp/fix-bug", call[4])
	assert.Equal(t, "abc123def456", call[5])
}

func initGitRepo(dir string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644); err != nil {
		return err
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = dir
	return cmd.Run()
}
