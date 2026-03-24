package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	tmpDir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run(), "git init")

	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644))

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	require.NoError(t, cmd.Run())

	return tmpDir
}

func TestWorktreeHooks_ExecutesPostCreateHooks(t *testing.T) {
	repo := initTestRepo(t)

	syncCfg := `mode: symlink
files: []
hooks:
  post_create:
    - 'echo "HOOK_EXECUTED"'
`
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git-ctx-sync.yaml"), []byte(syncCfg), 0644))

	worktreePath := filepath.Join(filepath.Dir(repo), "test-wt-hook")
	cmd := exec.Command(filepath.Join("/home/lvluu/git-profile", "git-ctx-test"), "worktree", "add", worktreePath)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(out))
	require.NoError(t, err, "worktree add should succeed")

	assert.Contains(t, string(out), "HOOK_EXECUTED", "hook should execute and output message")
	assert.Contains(t, string(out), "Worktree created at", "should confirm worktree creation")

	os.RemoveAll(worktreePath)
}

func TestWorktreeHooks_GlobalAndRepoHooks(t *testing.T) {
	repo := initTestRepo(t)

	syncCfg := `mode: symlink
files: []
hooks:
  post_create:
    - 'echo "REPO_HOOK"'
`
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git-ctx-sync.yaml"), []byte(syncCfg), 0644))

	worktreePath := filepath.Join(filepath.Dir(repo), "test-wt-both")
	cmd := exec.Command(filepath.Join("/home/lvluu/git-profile", "git-ctx-test"), "worktree", "add", worktreePath)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_CTX_WORKTREE_HOOKS=echo 'GLOBAL_HOOK'")
	out, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(out))
	require.NoError(t, err)

	assert.Contains(t, string(out), "REPO_HOOK", "repo hook should execute")
}

func TestWorktreeHooks_SkipWithFlag(t *testing.T) {
	repo := initTestRepo(t)

	syncCfg := `mode: symlink
files: []
hooks:
  post_create:
    - 'echo "SHOULD_NOT_RUN"'
`
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git-ctx-sync.yaml"), []byte(syncCfg), 0644))

	worktreePath := filepath.Join(filepath.Dir(repo), "test-wt-no-hooks")
	cmd := exec.Command(filepath.Join("/home/lvluu/git-profile", "git-ctx-test"), "worktree", "add", worktreePath, "--no-hooks")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(out))
	require.NoError(t, err)

	assert.NotContains(t, string(out), "SHOULD_NOT_RUN", "hook should not execute with --no-hooks")
	assert.Contains(t, string(out), "Worktree created at", "worktree should still be created")
}

func TestWorktreeHooks_StopsOnFailure(t *testing.T) {
	repo := initTestRepo(t)

	syncCfg := `mode: symlink
files: []
hooks:
  post_create:
    - 'echo "FIRST"'
    - 'exit 1'
    - 'echo "NEVER_RUN"'
`
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git-ctx-sync.yaml"), []byte(syncCfg), 0644))

	worktreePath := filepath.Join(filepath.Dir(repo), "test-wt-fail")
	cmd := exec.Command(filepath.Join("/home/lvluu/git-profile", "git-ctx-test"), "worktree", "add", worktreePath)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(out))

	assert.Error(t, err, "should fail when hook fails")
	assert.Contains(t, string(out), "FIRST", "first hook should execute")
	assert.NotContains(t, string(out), "NEVER_RUN", "third hook should not run")
}

func TestWorktreeHooks_EnvironmentVariables(t *testing.T) {
	repo := initTestRepo(t)

	syncCfg := `mode: symlink
files: []
hooks:
  post_create:
    - 'echo "BRANCH=$GIT_CTX_WORKTREE_BRANCH"'
    - 'echo "PATH=$GIT_CTX_WORKTREE_PATH"'
    - 'echo "REPO=$GIT_CTX_REPO_ROOT"'
`
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".git-ctx-sync.yaml"), []byte(syncCfg), 0644))

	worktreePath := filepath.Join(filepath.Dir(repo), "test-wt-env")
	cmd := exec.Command(filepath.Join("/home/lvluu/git-profile", "git-ctx-test"), "worktree", "add", worktreePath)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	t.Logf("Output: %s", string(out))
	require.NoError(t, err)

	assert.Contains(t, string(out), "BRANCH=test-wt-env")
	assert.Contains(t, string(out), "PATH="+worktreePath)
	assert.Contains(t, string(out), "REPO="+repo)
}
