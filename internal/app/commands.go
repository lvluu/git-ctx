// Package app wires together the CLI commands.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	profileCmd.AddCommand(buildSwitchCmd(mgr, g))
	profileCmd.AddCommand(buildAutoCmd(mgr, g, appCfg))
	profileCmd.AddCommand(buildExportCmd(mgr))
	profileCmd.AddCommand(buildImportCmd(mgr))
	profileCmd.AddCommand(buildDiffCmd(mgr))

	return profileCmd
}

func buildListCmd(mgr *profile.Manager, g git.Runner) *cobra.Command {
	var verbose, jsonOut bool

	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List all saved Git profiles",
		Run: func(cmd *cobra.Command, args []string) {
			if len(mgr.Profiles) == 0 {
				if jsonOut {
					fmt.Println("[]")
				} else {
					fmt.Println("No profiles found. Use 'git ctx profile add' to create a profile.")
				}
				return
			}

			activeName, activeEmail, _ := git.GetActiveProfile(g)

			if jsonOut {
				printProfilesJSON(mgr, activeName, activeEmail)
				return
			}

			if verbose {
				printProfilesVerbose(mgr, activeName, activeEmail)
				return
			}

			// Default: compact list.
			for name, p := range mgr.Profiles {
				raw := p
				resolved := p
				if raw.Extends != "" {
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
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed table with metadata")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output profiles as JSON")
	return cmd
}

// ProfileRow holds the data for one verbose table row.
type ProfileRow struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	SigningKey string `json:"signingKey,omitempty"`
	Template   string `json:"template,omitempty"`
	LastUsed   string `json:"lastUsed,omitempty"`
	Active     bool   `json:"active"`
}

func printProfilesVerbose(mgr *profile.Manager, activeName, activeEmail string) {
	// Collect and sort profile names for stable output.
	var names []string
	for name := range mgr.Profiles {
		names = append(names, name)
	}

	// Column widths (pre-computed for consistent padding).
	const (
		colNAME       = "NAME"
		colEMAIL      = "EMAIL"
		colSIGNINGKEY = "SIGNING KEY"
		colTEMPLATE   = "TEMPLATE"
		colACTIVE     = "ACTIVE"
		colLASTUSED   = "LAST USED"
	)

	// We need the effective widths based on content.
	type row struct{ name, email, key, tmpl, active, lastUsed string }
	rows := make([]row, 0, len(names))

	maxName, maxEmail, maxKey, maxTmpl := len(colNAME), len(colEMAIL), len(colSIGNINGKEY), len(colTEMPLATE)

	for _, name := range names {
		p, _, _ := mgr.Get(name)
		raw, _ := mgr.GetRaw(name)
		active := p.Name == activeName && p.Email == activeEmail
		activeStr := "-"
		if active {
			activeStr = "*"
		}
		lastUsed := raw.LastUsed
		if lastUsed == "" {
			lastUsed = "-"
		}
		tmplStr := "-"
		if raw.Extends != "" {
			tmplStr = raw.Extends
		}
		keyStr := "-"
		if p.Signing.Key != "" {
			keyStr = p.Signing.Key
		}
		rows = append(rows, row{
			name:     name,
			email:    p.Email,
			key:      keyStr,
			tmpl:     tmplStr,
			active:   activeStr,
			lastUsed: lastUsed,
		})
		if l := len(name); l > maxName {
			maxName = l
		}
		if l := len(p.Email); l > maxEmail {
			maxEmail = l
		}
		if l := len(keyStr); l > maxKey {
			maxKey = l
		}
		if l := len(tmplStr); l > maxTmpl {
			maxTmpl = l
		}
	}

	pad := func(s string, width int) string {
		return s + spaces(max(0, width-len(s)))
	}
	sep := func() {
		fmt.Print("  ")
	}

	// Header.
	fmt.Print(pad("", 2))
	sep()
	fmt.Print(pad(colNAME, maxName))
	sep()
	fmt.Print(pad(colEMAIL, maxEmail))
	sep()
	fmt.Print(pad(colSIGNINGKEY, maxKey))
	sep()
	fmt.Print(pad(colTEMPLATE, maxTmpl))
	sep()
	fmt.Print(colACTIVE)
	sep()
	fmt.Println(colLASTUSED)

	// Divider.
	fmt.Print(pad("", 2))
	sep()
	fmt.Print(pad("", maxName))
	sep()
	fmt.Print(pad("", maxEmail))
	sep()
	fmt.Print(pad("", maxKey))
	sep()
	fmt.Print(pad("", maxTmpl))
	sep()
	fmt.Print(pad("", len(colACTIVE)))
	sep()
	fmt.Println(strings.Repeat("-", len(colLASTUSED)))

	// Rows.
	for _, r := range rows {
		prefix := "  "
		fmt.Print(prefix)
		sep()
		fmt.Print(pad(r.name, maxName))
		sep()
		fmt.Print(pad(r.email, maxEmail))
		sep()
		fmt.Print(pad(r.key, maxKey))
		sep()
		fmt.Print(pad(r.tmpl, maxTmpl))
		sep()
		fmt.Print(pad(r.active, len(colACTIVE)))
		sep()
		fmt.Println(r.lastUsed)
	}
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func printProfilesJSON(mgr *profile.Manager, activeName, activeEmail string) {
	var rows []ProfileRow
	for name := range mgr.Profiles {
		p, _, _ := mgr.Get(name)
		raw, _ := mgr.GetRaw(name)
		active := p.Name == activeName && p.Email == activeEmail
		r := ProfileRow{
			Name:       name,
			Email:      p.Email,
			SigningKey: p.Signing.Key,
			Active:     active,
		}
		if raw.Extends != "" {
			r.Template = raw.Extends
		}
		if raw.LastUsed != "" {
			r.LastUsed = raw.LastUsed
		}
		rows = append(rows, r)
	}
	// Sort by name for stable output.
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	data, _ := json.MarshalIndent(rows, "", "  ")
	fmt.Println(string(data))
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
	var applyDryRun, applyDiff, applyQuiet bool

	cmd := &cobra.Command{
		Use:   "apply [profile-name]",
		Short: "Apply a specific Git profile",
		Args:  cobra.RangeArgs(0, 1),
		Run: func(cmd *cobra.Command, args []string) {
			var selected string
			if len(args) == 1 {
				selected = args[0]
			} else {
				var names []string
				for name := range mgr.Profiles {
					names = append(names, name)
				}
				prompt := promptui.Select{
					Label: "Select profile to apply",
					Items: names,
				}
				_, s, err := prompt.Run()
				if err != nil {
					fmt.Println("Cancelled.")
					return
				}
				selected = s
			}

			p, ok, err := mgr.Get(selected)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			if !ok {
				fmt.Printf("Profile '%s' not found.\n", selected)
				return
			}

			// Dry-run / diff mode: no config changes
			if applyDryRun || applyDiff {
				diffs, err := git.DiffProfile(g, "", "", p.Name, p.Email, p.Signing.Key)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
				if len(diffs) == 0 {
					if !applyQuiet {
						fmt.Println("[DRY RUN] No changes needed; user.name/user.email already match.")
					}
					return
				}
				if applyDryRun {
					if !applyQuiet {
						fmt.Print(git.FormatDryRun(diffs))
						fmt.Println()
					}
				} else {
					fmt.Print(git.FormatDiff(diffs))
					fmt.Println()
				}
				return
			}

			// Real apply
			if _, err := git.ApplyProfile(g, "", "", p.Name, p.Email, p.Signing.Key, true); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			if !applyQuiet {
				fmt.Printf("Profile '%s' applied successfully!\n", selected)
			}
		},
	}
	cmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Show what would be changed; don't modify config")
	cmd.Flags().BoolVar(&applyDiff, "diff", false, "Show changes as a git-style diff; don't modify config")
	cmd.Flags().BoolVar(&applyQuiet, "quiet", false, "Suppress output")
	return cmd
}

func buildSwitchCmd(mgr *profile.Manager, g git.Runner) *cobra.Command {
	return &cobra.Command{
		Use:   "switch [profile-name]",
		Short: "Switch to a profile (interactive fuzzy search, or non-interactive with argument)",
		Aliases: []string{"use"},
		Run: func(cmd *cobra.Command, args []string) {
			activeName, activeEmail, err := git.GetActiveProfile(g)
			if err != nil {
				fmt.Println("Error retrieving active profile:", err)
				os.Exit(1)
			}

			var selected string
			var ok bool

			if len(args) > 0 {
				// Non-interactive: apply the named profile directly
				selected = args[0]
				if _, exists := mgr.Profiles[selected]; !exists {
					fmt.Printf("Profile '%s' not found. Available profiles:\n", selected)
					for name := range mgr.Profiles {
						fmt.Printf("- %s\n", name)
					}
					os.Exit(1)
				}
				ok = true
			} else {
				// Build list items for the interactive picker
				var items []ui.ProfileListItem
				for name, raw := range mgr.Profiles {
					resolved, _, _ := mgr.Get(name)
					isActive := resolved.Name == activeName && resolved.Email == activeEmail
					displayName := name
					if raw.Extends != "" {
						displayName = fmt.Sprintf("%s (extends: %s)", name, raw.Extends)
					}
					items = append(items, ui.ProfileListItem{
						Name:        name,
						DisplayName: displayName,
						Email:       resolved.Email,
						IsActive:    isActive,
					})
				}
				selected, ok, err = ui.InteractiveProfilePicker(items, activeName, activeEmail)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					os.Exit(1)
				}
				if !ok {
					fmt.Println("Switch cancelled.")
					return
				}
			}

			p, _, err := mgr.Get(selected)
			if err != nil {
				fmt.Printf("Error loading profile: %v\n", err)
				os.Exit(1)
			}
			if _, err := git.ApplyProfile(g, "", "", p.Name, p.Email, p.Signing.Key, true); err != nil {
				fmt.Printf("Error applying profile: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Switched to profile '%s'.\n", selected)
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
				GitRunner:     g,
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
	return &cobra.Command{
		Use:   "export [output-file]",
		Short: "Export Git profiles to a JSON file",
		Run: func(cmd *cobra.Command, args []string) {
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
}

func buildImportCmd(mgr *profile.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "import <input-file>",
		Short: "Import Git profiles from a JSON file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := importProfiles(mgr, args[0]); err != nil {
				fmt.Println("Import failed:", err)
				os.Exit(1)
			}
		},
	}
}

func buildDiffCmd(mgr *profile.Manager) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "diff <profile-a> <profile-b>",
		Short: "Show the diff between two profiles",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			nameA, nameB := args[0], args[1]

			pA, okA, errA := mgr.Get(nameA)
			if errA != nil {
				fmt.Printf("Error loading profile %q: %v\n", nameA, errA)
				os.Exit(1)
			}
			if !okA {
				fmt.Printf("Profile %q not found.\n", nameA)
				os.Exit(1)
			}

			pB, okB, errB := mgr.Get(nameB)
			if errB != nil {
				fmt.Printf("Error loading profile %q: %v\n", nameB, errB)
				os.Exit(1)
			}
			if !okB {
				fmt.Printf("Profile %q not found.\n", nameB)
				os.Exit(1)
			}

			delta := profile.DiffProfiles(pA, pB)

			if jsonOutput {
				type diffEntry struct {
					Key    string `json:"key"`
					From   string `json:"from"`
					To     string `json:"to"`
				}
				var out []diffEntry
				for key, pair := range delta {
					out = append(out, diffEntry{Key: key, From: pair[0], To: pair[1]})
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					fmt.Println("JSON encode error:", err)
					os.Exit(1)
				}
				return
			}

			if len(delta) == 0 {
				fmt.Printf("Profiles %q and %q are identical.\n", nameA, nameB)
				return
			}

			for key, pair := range delta {
				from := pair[0]
				to := pair[1]
				if from == "" {
					fmt.Printf("[+] %s %s\n", key, to)
				} else if to == "" {
					fmt.Printf("[-] %s %s\n", key, from)
				} else {
					fmt.Printf("[~] %s\n", key)
					fmt.Printf("    [-] %s\n    [+] %s\n", from, to)
				}
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output diff as JSON array")
	return cmd
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
func BuildDoctorCmd(cfg config.AppConfig, mgr *profile.Manager, g git.Runner) *cobra.Command {
	var doFix, dryRun bool

	cmd := &cobra.Command{
		Use: "doctor",
		Short: "Diagnose git-ctx configuration issues",
		Long: `Diagnose git-ctx configuration issues.

With --fix, auto-repairs detected issues:
  • Missing shell init → appends to ~/.bashrc and ~/.zshrc
  • Corrupted profiles file → backs up and re-initializes
  • Directory rule references missing profile → creates stub profile

With --dry-run, shows what would be repaired without making changes.
`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("git-ctx doctor")
			fmt.Println()
			results := runDoctorChecks(cfg, mgr)
			printDoctorResults(results)

			if !doFix {
				for _, r := range results {
					if !r.OK {
						os.Exit(1)
					}
				}
				return
			}

			// Collect repo root for context.
			repoRoot, _, _ := git.FindRepoRoot(g)
			gitBin, _ := exec.LookPath("git")

			outcomes := runDoctorRepair(results, cfg, repoRoot, gitBin, mgr, dryRun)
			if len(outcomes) == 0 {
				fmt.Println("\nNo automatic repairs available for the issues above.")
				os.Exit(1)
				return
			}

			fmt.Println()
			if dryRun {
				fmt.Println("Dry run — no changes made.")
			} else {
				fmt.Println("Repairs applied.")
			}
			hadFailure := false
			for checkName, outcome := range outcomes {
				prefix := "  [fix] "
				if dryRun {
					prefix = "  [fix?] "
				}
				if outcome.Success {
					fmt.Printf("%s%s: OK — %s\n", prefix, checkName, outcome.Summary)
				} else {
					fmt.Printf("%s%s: FAILED — %s\n", prefix, checkName, outcome.Summary)
					hadFailure = true
				}
				if outcome.BackupPath != "" {
					fmt.Printf("         Backup: %s\n", outcome.BackupPath)
				}
				if dryRun && len(outcome.DryRunHints) > 0 {
					for _, h := range outcome.DryRunHints {
						fmt.Printf("         → %s\n", h)
					}
				}
			}
			if hadFailure {
				os.Exit(1)
			}
		},
	}
	cmd.Flags().BoolVar(&doFix, "fix", false, "Automatically repair detected issues")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be repaired without making changes")
	return cmd
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

func importProfiles(mgr *profile.Manager, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var imported map[string]profile.Profile
	if err := json.Unmarshal(data, &imported); err != nil {
		return err
	}
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
