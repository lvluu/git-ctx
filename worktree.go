package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// buildWorktreeCmd constructs the `worktree` subcommand group.
func buildWorktreeCmd(appCfg AppConfig, git GitRunner) *cobra.Command {
	worktreeCmd := &cobra.Command{
		Use:   "worktree",
		Short: "Manage git worktrees with file sync",
		Long: `Manage git worktrees with file sync.

File sync is configured via .git-ctx-sync.yaml in the repo root:

  mode: symlink   # symlink (default) or copy
  files:
    - .env
    - .vscode/settings.json
  hooks:
    post_create:
      - bun install

- mode is optional; falls back to worktree.default_mode in ~/.git-ctx.yaml
- files are relative to the repo root
- hooks run after worktree creation (global hooks from ~/.git-ctx.yaml run first)
- .git-ctx-sync.yaml is local-only (add to .gitignore)`,
	}

	// ── worktree ls ──────────────────────────────────────────────────────
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List git worktrees",
		Run: func(cmd *cobra.Command, args []string) {
			out, err := git.Output("", "worktree", "list")
			if err != nil {
				fmt.Println("Error listing worktrees:", err)
				os.Exit(1)
			}
			fmt.Print(string(out))
		},
	}

	// ── worktree add ─────────────────────────────────────────────────────
	var addCopy, noHooks bool
	addCmd := &cobra.Command{
		Use:   "add <path> [<commit-ish>]",
		Short: "Add a worktree and sync configured files into it",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			wtPath := args[0]
			branchName := strings.TrimSuffix(filepath.Base(wtPath), "/")

			gitArgs := []string{"worktree", "add", "-b", branchName, wtPath}
			if len(args) == 2 {
				gitArgs = append(gitArgs, args[1])
			}

			if err := git.Run("", gitArgs...); err != nil {
				fmt.Println("git worktree add failed:", err)
				os.Exit(1)
			}

			absWTPath, err := filepath.Abs(wtPath)
			if err != nil {
				fmt.Println("Error resolving worktree path:", err)
				os.Exit(1)
			}

			repoRoot, inRepo, err := findRepoRoot(git)
			if err != nil {
				fmt.Println("Error finding repo root:", err)
				os.Exit(1)
			}
			if !inRepo {
				fmt.Println("Error: not inside a git repository")
				os.Exit(1)
			}

			syncCfgPath := filepath.Join(repoRoot, ".git-ctx-sync.yaml")
			syncCfg, err := loadSyncConfig(syncCfgPath, appCfg.Worktree.DefaultMode)
			if err != nil {
				fmt.Println("Error loading sync config:", err)
				os.Exit(1)
			}

			if len(syncCfg.Files) > 0 {
				warnings, err := syncFiles(syncCfg, repoRoot, absWTPath, addCopy)
				if err != nil {
					fmt.Println("Sync failed:", err)
					os.Exit(1)
				}
				for _, w := range warnings {
					fmt.Println("Warning:", w)
				}
			}

			if !noHooks {
				runner := &ExecHookRunner{Stdout: os.Stdout, Stderr: os.Stderr}
				allHooks := append(appCfg.Worktree.Hooks.PostCreate, syncCfg.Hooks.PostCreate...)
				if err := runHooks(runner, allHooks, absWTPath, branchName, repoRoot); err != nil {
					fmt.Println("Hook failed:", err)
					os.Exit(1)
				}
			}

			fmt.Printf("Worktree created at %s\n", absWTPath)
		},
	}
	addCmd.Flags().BoolVar(&addCopy, "copy", false, "Copy files instead of symlinking")
	addCmd.Flags().BoolVar(&noHooks, "no-hooks", false, "Skip post-create hooks")

	// ── worktree sync ─────────────────────────────────────────────────────
	var syncCopy bool
	syncCmd := &cobra.Command{
		Use:   "sync [<path>]",
		Short: "Sync configured files into one or all worktrees",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 1 {
				absPath, err := filepath.Abs(args[0])
				if err != nil {
					fmt.Println("Error resolving path:", err)
					os.Exit(1)
				}
				warnings, err := runSync(appCfg, git, absPath, syncCopy)
				if err != nil {
					fmt.Println("Sync failed:", err)
					os.Exit(1)
				}
				for _, w := range warnings {
					fmt.Println("Warning:", w)
				}
				fmt.Printf("Synced files to %s\n", absPath)
				return
			}

			// No path given — sync all non-main worktrees.
			worktrees, err := listWorktreePaths(git)
			if err != nil {
				fmt.Println("Error listing worktrees:", err)
				os.Exit(1)
			}
			if len(worktrees) <= 1 {
				fmt.Println("No additional worktrees found.")
				return
			}
			for _, wt := range worktrees[1:] {
				warnings, err := runSync(appCfg, git, wt, syncCopy)
				if err != nil {
					fmt.Printf("Sync failed for %s: %v\n", wt, err)
					continue
				}
				for _, w := range warnings {
					fmt.Printf("Warning (%s): %s\n", wt, w)
				}
				fmt.Printf("Synced files to %s\n", wt)
			}
		},
	}
	syncCmd.Flags().BoolVar(&syncCopy, "copy", false, "Copy files instead of symlinking")

	worktreeCmd.AddCommand(lsCmd, addCmd, syncCmd)
	return worktreeCmd
}

// runSync loads .git-ctx-sync.yaml from the repo root and syncs files to dstWorktree.
func runSync(appCfg AppConfig, git GitRunner, dstWorktree string, copyOverride bool) ([]string, error) {
	repoRoot, inRepo, err := findRepoRoot(git)
	if err != nil {
		return nil, err
	}
	if !inRepo {
		return nil, fmt.Errorf("not inside a git repository")
	}

	syncCfgPath := filepath.Join(repoRoot, ".git-ctx-sync.yaml")
	cfg, err := loadSyncConfig(syncCfgPath, appCfg.Worktree.DefaultMode)
	if err != nil {
		return nil, err
	}

	if len(cfg.Files) == 0 {
		return nil, nil
	}

	return syncFiles(cfg, repoRoot, dstWorktree, copyOverride)
}

// listWorktreePaths returns absolute paths of all worktrees by parsing
// `git worktree list --porcelain`.
func listWorktreePaths(git GitRunner) ([]string, error) {
	out, err := git.Output("", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}
