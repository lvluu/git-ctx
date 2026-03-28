// Package app wires together the CLI commands.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lvluu/git-ctx/internal/config"
	"github.com/lvluu/git-ctx/internal/git"
	"github.com/lvluu/git-ctx/internal/profile"
	"github.com/lvluu/git-ctx/internal/ui"
	"github.com/lvluu/git-ctx/internal/worktree"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

// Version info (set via ldflags).
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// ShellInitScript returns the shell hook snippet for auto-applying profiles.
func ShellInitScript() string {
	return `# git-ctx shell integration
# Add to ~/.bashrc or ~/.zshrc:
#   eval "$(git ctx shell-init)"

# gc is a short alias for git-ctx
alias gc="git-ctx"

__git_ctx_auto() {
    git-ctx profile auto --silent 2>/dev/null
}

# bash
if [ -n "$BASH_VERSION" ]; then
    PROMPT_COMMAND="${PROMPT_COMMAND:+${PROMPT_COMMAND};}__git_ctx_auto"
fi

# zsh
if [ -n "$ZSH_VERSION" ]; then
    autoload -U add-zsh-hook
    add-zsh-hook chpwd __git_ctx_auto
    __git_ctx_auto  # run once on shell start
fi
`
}

// BuildProfileCmd constructs the profile subcommand with all its subcommands.
func BuildProfileCmd(mgr *profile.Manager, g git.Runner, appCfg config.AppConfig) *cobra.Command {
	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage git profiles",
	}

	profileCmd.AddCommand(buildListCmd(mgr, g))
	profileCmd.AddCommand(buildAddCmd(mgr))
	profileCmd.AddCommand(buildEditCmd(mgr))
	profileCmd.AddCommand(buildRemoveCmd(mgr))
	profileCmd.AddCommand(buildApplyCmd(mgr, g))
	profileCmd.AddCommand(buildAutoCmd(mgr, g, appCfg))
	profileCmd.AddCommand(buildExportCmd(mgr))
	profileCmd.AddCommand(buildImportCmd(mgr))

	return profileCmd
}

func buildListCmd(mgr *profile.Manager, g git.Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all saved Git profiles",
		Run: func(cmd *cobra.Command, args []string) {
			if len(mgr.Profiles) == 0 {
				fmt.Println("No profiles found. Use 'git ctx profile add' to create a profile.")
				return
			}
			activeName, activeEmail, err := git.GetActiveProfile(g)
			if err != nil {
				fmt.Println("Error retrieving active profile:", err)
				return
			}
			for name, p := range mgr.Profiles {
				raw := p
				resolved := p
				if p.Extends != "" {
					resolved, _, _ = mgr.Get(name)
				}
				activeMarker := ""
				if resolved.Name == activeName && resolved.Email == activeEmail {
					activeMarker = " (active)"
				}
				extendsMarker := ""
				if raw.Extends != "" {
					extendsMarker = fmt.Sprintf(" (extends: %s)", raw.Extends)
				}
				fmt.Printf("💻 Profile: %s%s%s\n", name, activeMarker, extendsMarker)
				fmt.Printf("  🖖 Name:  %s\n", resolved.Name)
				fmt.Printf("  📧 Email: %s\n", resolved.Email)
				if resolved.Signing.Key != "" {
					fmt.Printf("  🔑 Signing Key: %s\n", resolved.Signing.Key)
				}
				fmt.Println()
			}
		},
	}
}

func buildAddCmd(mgr *profile.Manager) *cobra.Command {
	var extendsTemplate string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			prompt := promptui.Prompt{
				Label: "Enter profile name",
				Validate: func(input string) error {
					if input == "" {
						return fmt.Errorf("profile name cannot be empty")
					}
					if _, exists := mgr.Profiles[input]; exists {
						return fmt.Errorf("profile '%s' already exists", input)
					}
					return nil
				},
			}
			profileName, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			p := ui.PromptProfileDetails(nil)
			p.Extends = extendsTemplate
			mgr.Profiles[profileName] = p
			mgr.Save()
			fmt.Printf("Profile '%s' added successfully!\n", profileName)
		},
	}
	cmd.Flags().StringVar(&extendsTemplate, "extends", "", "Template name to extend")
	return cmd
}

func buildEditCmd(mgr *profile.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			var names []string
			for name := range mgr.Profiles {
				names = append(names, name)
			}
			prompt := promptui.Select{
				Label: "Select profile to edit",
				Items: names,
			}
			_, selected, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			existing := mgr.Profiles[selected]
			updated := ui.PromptProfileDetails(&existing)
			mgr.Profiles[selected] = updated
			mgr.Save()
			fmt.Printf("Profile '%s' updated successfully!\n", selected)
		},
	}
}

func buildRemoveCmd(mgr *profile.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "rm",
		Short: "Remove a Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			var names []string
			for name := range mgr.Profiles {
				names = append(names, name)
			}
			prompt := promptui.Select{
				Label: "Select profile to remove",
				Items: names,
			}
			_, selected, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			confirm := promptui.Prompt{
				Label:     fmt.Sprintf("Are you sure you want to remove profile '%s'", selected),
				IsConfirm: true,
			}
			if _, err := confirm.Run(); err != nil {
				fmt.Println("Removal cancelled.")
				return
			}
			delete(mgr.Profiles, selected)
			mgr.Save()
			fmt.Printf("Profile '%s' removed successfully!\n", selected)
		},
	}
}

func buildApplyCmd(mgr *profile.Manager, g git.Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "Apply a specific Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			var names []string
			for name := range mgr.Profiles {
				names = append(names, name)
			}
			prompt := promptui.Select{
				Label: "Select profile to apply",
				Items: names,
			}
			_, selected, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			p, ok, err := mgr.Get(selected)
			if err != nil {
				fmt.Printf("Error applying profile: %v\n", err)
				return
			}
			if !ok {
				fmt.Printf("Profile '%s' not found.\n", selected)
				return
			}
			if _, err := git.ApplyProfile(g, "", "", p.Name, p.Email, p.Signing.Key, true); err != nil {
				fmt.Printf("Error applying profile: %v\n", err)
				return
			}
			fmt.Printf("Profile '%s' applied successfully!\n", selected)
		},
	}
}

func buildAutoCmd(mgr *profile.Manager, g git.Runner, appCfg config.AppConfig) *cobra.Command {
	var autoForce bool
	var autoSilent bool

	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Automatically apply profile from .gitprofilerc or directory rules",
		Run: func(cmd *cobra.Command, args []string) {
			resolver := profile.Resolver{
				GetRepoRoot:    func() (string, bool, error) { return git.FindRepoRoot(g) },
				GetHomeDir:     os.UserHomeDir,
				ReadFile:       os.ReadFile,
				FileExists:     func(path string) bool { _, err := os.Stat(path); return err == nil },
				DirectoryRules: convertDirRules(appCfg.DirectoryRules),
				GetCurrentDir:  os.Getwd,
			}
			res, err := resolver.Resolve()
			if err != nil {
				// Real errors should always be reported, even with --silent
				fmt.Println("Auto apply failed:", err)
				os.Exit(1)
			}
			// Silent mode only suppresses "no match" (empty profile key)
			if res.ProfileKey == "" {
				if autoSilent {
					return
				}
				fmt.Println("No profile found to apply.")
				os.Exit(1)
			}
			p, ok, err := mgr.Get(res.ProfileKey)
			if err != nil {
				fmt.Printf("Auto apply failed: %v\n", err)
				os.Exit(1)
			}
			if !ok {
				// Profile key resolved but doesn't exist - report error
				fmt.Printf("Auto apply failed: profile '%s' not found. Available profiles:\n", res.ProfileKey)
				for k := range mgr.Profiles {
					fmt.Printf("- %s\n", k)
				}
				os.Exit(1)
			}
			changed, err := git.ApplyProfile(g, res.WorkDir, res.ScopeFlag, p.Name, p.Email, p.Signing.Key, autoForce)
			if err != nil {
				fmt.Println("Auto apply failed:", err)
				os.Exit(1)
			}
			if !autoSilent {
				if changed {
					fmt.Printf("Applied profile '%s' from %s (%s).\n", res.ProfileKey, res.RCPath, trimScopeFlag(res.ScopeFlag))
				} else {
					fmt.Printf("No changes: %s config already sets user.name/user.email (use --force to overwrite).\n", trimScopeFlag(res.ScopeFlag))
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&autoForce, "force", "f", false, "Overwrite existing user.name/user.email")
	cmd.Flags().BoolVar(&autoSilent, "silent", false, "Exit 0 without output when no profile matches (for shell hooks)")
	return cmd
}

func buildExportCmd(mgr *profile.Manager) *cobra.Command {
	var gistExport, gistPublic bool

	cmd := &cobra.Command{
		Use:   "export [output-file]",
		Short: "Export Git profiles to a JSON file or GitHub Gist",
		Run: func(cmd *cobra.Command, args []string) {
			if gistExport {
				token, err := getGHAuthToken()
				if err != nil {
					fmt.Println("Export failed:", err)
					os.Exit(1)
				}
				url, err := mgr.ExportToGist(gistPublic, http.DefaultClient, token)
				if err != nil {
					fmt.Println("Export failed:", err)
					os.Exit(1)
				}
				fmt.Printf("Profiles exported to: %s\n", url)
				return
			}
			// File mode.
			var outputPath string
			if len(args) > 0 {
				outputPath = args[0]
			}
			if outputPath == "" {
				homeDir, _ := os.UserHomeDir()
				outputPath = filepath.Join(homeDir, "git-ctx-profiles-export.json")
			}
			if filepath.Ext(outputPath) != ".json" {
				outputPath += ".json"
			}
			if err := exportProfiles(mgr.Profiles, outputPath); err != nil {
				fmt.Println("Export failed:", err)
				os.Exit(1)
			}
			fmt.Printf("Profiles exported to: %s\n", outputPath)
		},
	}
	cmd.Flags().BoolVar(&gistExport, "gist", false, "Export to a GitHub Gist (requires gh auth)")
	cmd.Flags().BoolVar(&gistPublic, "public", false, "Make Gist public (default: secret)")
	return cmd
}

func buildImportCmd(mgr *profile.Manager) *cobra.Command {
	var merge bool

	cmd := &cobra.Command{
		Use:   "import <input>",
		Short: "Import Git profiles from a JSON file or GitHub Gist",
		Long: `Import Git profiles from a JSON file or a GitHub Gist URL.

When given a Gist URL (https://gist.github.com/...), profiles are imported
from that Gist. When given a file path, profiles are imported from that file.

Use --merge to add new profiles without replacing existing ones.
Without --merge, a prompt asks whether to merge or replace.`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			input := args[0]
			if isGistURL(input) {
				token, err := getGHAuthToken()
				if err != nil {
					fmt.Println("Import failed:", err)
					os.Exit(1)
				}
				if err := mgr.ImportFromGist(input, merge, http.DefaultClient, token); err != nil {
					fmt.Println("Import failed:", err)
					os.Exit(1)
				}
				fmt.Printf("Profiles imported from: %s\n", input)
				return
			}
			// File import.
			if err := importProfiles(mgr, input, merge); err != nil {
				fmt.Println("Import failed:", err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().BoolVar(&merge, "merge", false, "Merge new profiles without replacing existing ones")
	return cmd
}

// isGistURL reports whether input looks like a GitHub Gist URL.
func isGistURL(input string) bool {
	return strings.HasPrefix(input, "https://gist.github.com/") ||
		strings.HasPrefix(input, "http://gist.github.com/") ||
		strings.HasPrefix(input, "https://gist.githubusercontent.com/") ||
		strings.HasPrefix(input, "http://gist.githubusercontent.com/")
}

func buildWorktreeCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
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

	worktreeCmd.AddCommand(buildWorktreeLsCmd(g))
	worktreeCmd.AddCommand(buildWorktreeAddCmd(appCfg, g))
	worktreeCmd.AddCommand(buildWorktreeSyncCmd(appCfg, g))
	worktreeCmd.AddCommand(buildWorktreePushCmd(appCfg, g))
	worktreeCmd.AddCommand(buildWorktreePullCmd(appCfg, g))
	worktreeCmd.AddCommand(buildWorktreeWatchCmd(appCfg, g))

	return worktreeCmd
}

func buildWorktreeLsCmd(g git.Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List git worktrees",
		Run: func(cmd *cobra.Command, args []string) {
			out, err := g.Output("", "worktree", "list")
			if err != nil {
				fmt.Println("Error listing worktrees:", err)
				os.Exit(1)
			}
			fmt.Print(string(out))
		},
	}
}

func buildWorktreeAddCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
	var addCopy, noHooks bool

	cmd := &cobra.Command{
		Use:   "add <path> [<commit-ish>]",
		Short: "Add a worktree and sync configured files into it",
		Args:  cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			wtPath := args[0]
			branchName := trimSuffix(filepath.Base(wtPath), "/")

			gitArgs := []string{"worktree", "add", "-b", branchName, wtPath}
			if len(args) == 2 {
				gitArgs = append(gitArgs, args[1])
			}

			if err := g.Run("", gitArgs...); err != nil {
				fmt.Println("git worktree add failed:", err)
				os.Exit(1)
			}

			absWTPath, err := filepath.Abs(wtPath)
			if err != nil {
				fmt.Println("Error resolving worktree path:", err)
				os.Exit(1)
			}

			repoRoot, inRepo, err := git.FindRepoRoot(g)
			if err != nil {
				fmt.Println("Error finding repo root:", err)
				os.Exit(1)
			}
			if !inRepo {
				fmt.Println("Error: not inside a git repository")
				os.Exit(1)
			}

			syncCfgPath := filepath.Join(repoRoot, ".git-ctx-sync.yaml")
			syncCfg, err := worktree.LoadSyncConfig(syncCfgPath, appCfg.Worktree.DefaultMode)
			if err != nil {
				fmt.Println("Error loading sync config:", err)
				os.Exit(1)
			}

			if len(syncCfg.Files) > 0 {
				warnings, err := worktree.SyncFiles(syncCfg, repoRoot, absWTPath, addCopy)
				if err != nil {
					fmt.Println("Sync failed:", err)
					os.Exit(1)
				}
				for _, w := range warnings {
					fmt.Println("Warning:", w)
				}
			}

			if !noHooks {
				runner := &worktree.ExecHookRunner{Stdout: os.Stdout, Stderr: os.Stderr}
				allHooks := append(appCfg.Worktree.Hooks.PostCreate, syncCfg.Hooks.PostCreate...)
				if err := worktree.RunHooks(runner, allHooks, absWTPath, branchName, repoRoot); err != nil {
					fmt.Println("Hook failed:", err)
					os.Exit(1)
				}
			}

			fmt.Printf("Worktree created at %s\n", absWTPath)
		},
	}
	cmd.Flags().BoolVar(&addCopy, "copy", false, "Copy files instead of symlinking")
	cmd.Flags().BoolVar(&noHooks, "no-hooks", false, "Skip post-create hooks")
	return cmd
}

func buildWorktreeSyncCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
	var syncCopy bool

	cmd := &cobra.Command{
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
				warnings, err := worktree.RunSync(appCfg, g, absPath, syncCopy)
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

			paths, err := worktree.ListWorktreePaths(g)
			if err != nil {
				fmt.Println("Error listing worktrees:", err)
				os.Exit(1)
			}
			if len(paths) <= 1 {
				fmt.Println("No additional worktrees found.")
				return
			}
			hadFailure := false
			for _, wt := range paths[1:] {
				warnings, err := worktree.RunSync(appCfg, g, wt, syncCopy)
				if err != nil {
					fmt.Printf("Sync failed for %s: %v\n", wt, err)
					hadFailure = true
					continue
				}
				for _, w := range warnings {
					fmt.Printf("Warning (%s): %s\n", wt, w)
				}
				fmt.Printf("Synced files to %s\n", wt)
			}
			if hadFailure {
				os.Exit(1)
			}
		},
	}
	cmd.Flags().BoolVar(&syncCopy, "copy", false, "Copy files instead of symlinking")
	return cmd
}

// buildWorktreePushCmd pushes configured files from the repo root into a worktree.
func buildWorktreePushCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
	var pushCopy bool

	cmd := &cobra.Command{
		Use:   "push <worktree-path>",
		Short: "Push files from the primary worktree into a worktree",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			absPath, err := filepath.Abs(args[0])
			if err != nil {
				fmt.Println("Error resolving path:", err)
				os.Exit(1)
			}
			warnings, err := worktree.RunSyncPush(appCfg, g, absPath, pushCopy)
			if err != nil {
				fmt.Println("Push failed:", err)
				os.Exit(1)
			}
			for _, w := range warnings {
				fmt.Println("Warning:", w)
			}
			fmt.Printf("Pushed files to %s\n", absPath)
		},
	}
	cmd.Flags().BoolVar(&pushCopy, "copy", false, "Copy files instead of symlinking")
	return cmd
}

// buildWorktreePullCmd pulls configured files from a worktree back to the primary.
func buildWorktreePullCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
	var pullCopy bool

	cmd := &cobra.Command{
		Use:   "pull <worktree-path>",
		Short: "Pull files from a worktree into the primary worktree",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			absPath, err := filepath.Abs(args[0])
			if err != nil {
				fmt.Println("Error resolving path:", err)
				os.Exit(1)
			}
			warnings, err := worktree.RunSyncPull(appCfg, g, absPath, pullCopy)
			if err != nil {
				var conflictErr *worktree.ConflictError
				if errors.As(err, &conflictErr) {
					fmt.Println("Pull aborted due to conflict:", conflictErr.Error())
				} else {
					fmt.Println("Pull failed:", err)
				}
				os.Exit(1)
			}
			for _, w := range warnings {
				fmt.Println("Warning:", w)
			}
			fmt.Printf("Pulled files from %s\n", absPath)
		},
	}
	cmd.Flags().BoolVar(&pullCopy, "copy", false, "Copy files instead of symlinking")
	return cmd
}

// buildWorktreeWatchCmd starts a watch loop that continuously syncs files.
func buildWorktreeWatchCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
	var watchCopy bool

	cmd := &cobra.Command{
		Use:   "watch [worktree-path]",
		Short: "Watch and continuously sync files between worktrees",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repoRoot, inRepo, err := git.FindRepoRoot(g)
			if err != nil || !inRepo {
				fmt.Println("Error: not inside a git repository")
				os.Exit(1)
			}

			var dstRoot string
			if len(args) == 1 {
				dstRoot, _ = filepath.Abs(args[0])
			} else {
				paths, err := worktree.ListWorktreePaths(g)
				if err != nil || len(paths) <= 1 {
					fmt.Println("No additional worktrees found.")
					os.Exit(0)
				}
				dstRoot = paths[1] // sync to the first non-primary worktree
			}

			syncCfg, err := worktree.LoadSyncConfig(filepath.Join(repoRoot, ".git-ctx-sync.yaml"), appCfg.Worktree.DefaultMode)
			if err != nil || len(syncCfg.Files) == 0 {
				fmt.Println("No sync files configured in .git-ctx-sync.yaml")
				os.Exit(0)
			}

			fmt.Printf("Watching %s for changes (debounce: %v)\n",
				strings.Join(syncCfg.Files, ", "), worktree.DefaultWatchConfig.Debounce)
			fmt.Printf("Syncing to: %s\n", dstRoot)

			worktree.WatchLoop(syncCfg, repoRoot, dstRoot, watchCopy, worktree.DefaultWatchConfig,
				func(warnings []string) {
					for _, w := range warnings {
						fmt.Println("Warning:", w)
					}
					fmt.Println("Synced")
				},
				func(err error) {
					fmt.Println("Watch error:", err)
				},
			)
		},
	}
	cmd.Flags().BoolVar(&watchCopy, "copy", false, "Copy files instead of symlinking")
	return cmd
}


// BuildWorktreeCmd builds the worktree command group.
func BuildWorktreeCmd(appCfg config.AppConfig, g git.Runner) *cobra.Command {
	return buildWorktreeCmd(appCfg, g)
}

// BuildDoctorCmd builds the doctor command.
func BuildDoctorCmd(cfg config.AppConfig, mgr *profile.Manager) *cobra.Command {
	return &cobra.Command{
		Use: "doctor",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("git-ctx doctor")
			fmt.Println()
			results := runDoctorChecks(cfg, mgr)
			printDoctorResults(results)
			for _, r := range results {
				if !r.OK {
					os.Exit(1)
				}
			}
		},
	}
}

type doctorResult struct {
	Name   string
	OK     bool
	Detail string
}

func runDoctorChecks(cfg config.AppConfig, mgr *profile.Manager) []doctorResult {
	var results []doctorResult

	check := func(name string, ok bool, detail string) {
		results = append(results, doctorResult{Name: name, OK: ok, Detail: detail})
	}

	checkBinary := func() {
		_, err := exec.LookPath("git")
		if err == nil {
			check("git binary", true, "found")
		} else {
			check("git binary", false, "git not found in PATH")
		}
	}
	checkBinary()

	if _, err := os.Stat(cfg.ProfilesPath); os.IsNotExist(err) {
		check("profiles file", true, fmt.Sprintf("%s not yet created (will be created on first 'profile add')", cfg.ProfilesPath))
	} else {
		check("profiles file", true, cfg.ProfilesPath)
	}

	mode := cfg.Worktree.DefaultMode
	switch mode {
	case "", "symlink":
		display := mode
		if display == "" {
			display = "symlink (default)"
		}
		check("worktree mode", true, display)
	case "copy":
		check("worktree mode", true, mode)
	default:
		check("worktree mode", false, fmt.Sprintf("invalid mode %q (must be 'symlink' or 'copy')", mode))
	}

	if _, err := os.Stat(cfg.ProfilesPath); err == nil {
		for _, rule := range cfg.DirectoryRules {
			if _, ok := mgr.Profiles[rule.Profile]; !ok {
				check(fmt.Sprintf("directory rule '%s'", rule.Pattern), false, fmt.Sprintf("profile '%s' not found in %s", rule.Profile, cfg.ProfilesPath))
			} else {
				check(fmt.Sprintf("directory rule '%s'", rule.Pattern), true, fmt.Sprintf("→ profile '%s'", rule.Profile))
			}
		}
	}

	check("shell hook", true, `add 'eval "$(git ctx shell-init)"' to ~/.bashrc or ~/.zshrc`)

	return results
}

func printDoctorResults(results []doctorResult) {
	allOK := true
	for _, r := range results {
		icon := "✓"
		if !r.OK {
			icon = "✗"
			allOK = false
		}
		if r.Detail != "" {
			fmt.Printf("  [%s] %s: %s\n", icon, r.Name, r.Detail)
		} else {
			fmt.Printf("  [%s] %s\n", icon, r.Name)
		}
	}
	fmt.Println()
	if allOK {
		fmt.Println("All checks passed.")
	} else {
		issues := 0
		for _, r := range results {
			if !r.OK {
				issues++
			}
		}
		fmt.Printf("%d issue(s) found. Run 'git ctx init' to regenerate config.\n", issues)
	}
}

func convertDirRules(rules []config.DirectoryRule) []profile.DirectoryRule {
	result := make([]profile.DirectoryRule, len(rules))
	for i, r := range rules {
		result[i] = profile.DirectoryRule{Pattern: r.Pattern, Profile: r.Profile}
	}
	return result
}

func trimScopeFlag(flag string) string {
	if len(flag) > 2 && flag[:2] == "--" {
		return flag[2:]
	}
	return flag
}

func exportProfiles(profiles map[string]profile.Profile, path string) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func importProfiles(mgr *profile.Manager, path string, merge bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var imported map[string]profile.Profile
	if err := json.Unmarshal(data, &imported); err != nil {
		return err
	}
	if merge {
		for name, p := range imported {
			if _, exists := mgr.Profiles[name]; !exists {
				mgr.Profiles[name] = p
			}
		}
	} else {
		prompt := promptui.Select{
			Label: "Import Strategy",
			Items: []string{
				"Merge (Add new profiles, keep existing)",
				"Replace (Overwrite all existing profiles)",
			},
		}
		_, strategy, err := prompt.Run()
		if err != nil {
			return fmt.Errorf("import cancelled")
		}
		switch strategy {
		case "Merge (Add new profiles, keep existing)":
			for name, p := range imported {
				if _, exists := mgr.Profiles[name]; !exists {
					mgr.Profiles[name] = p
				}
			}
		case "Replace (Overwrite all existing profiles)":
			mgr.Profiles = imported
		}
	}
	mgr.Save()
	fmt.Printf("Profiles imported successfully. Total profiles: %d\n", len(mgr.Profiles))
	return nil
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// getGHAuthToken reads the GitHub authentication token from the gh CLI.
func getGHAuthToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get gh auth token: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
