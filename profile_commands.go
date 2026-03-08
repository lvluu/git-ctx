package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

// buildProfileCmd constructs the `profile` subcommand group.
func buildProfileCmd(cm *ConfigManager, git GitRunner, appCfg AppConfig) *cobra.Command {
	profileCmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage git profiles",
	}

	listCmd := &cobra.Command{
		Use:   "ls",
		Short: "List all saved Git profiles",
		Run: func(cmd *cobra.Command, args []string) {
			if len(cm.Profiles) == 0 {
				fmt.Println("No profiles found. Use 'git ctx profile add' to create a profile.")
				return
			}
			activeName, activeEmail, err := getActiveProfile()
			if err != nil {
				fmt.Println("Error retrieving active profile:", err)
				return
			}
			for name, profile := range cm.Profiles {
				activeMarker := ""
				if profile.Name == activeName && profile.Email == activeEmail {
					activeMarker = " (active)"
				}
				fmt.Printf("💻 Profile: %s%s\n", name, activeMarker)
				fmt.Printf("  🖖 Name:  %s\n", profile.Name)
				fmt.Printf("  📧 Email: %s\n", profile.Email)
				if profile.Signing.Key != "" {
					fmt.Printf("  🔑 Signing Key: %s\n", profile.Signing.Key)
				}
				fmt.Println()
			}
		},
	}

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			prompt := promptui.Prompt{
				Label: "Enter profile name",
				Validate: func(input string) error {
					if input == "" {
						return fmt.Errorf("profile name cannot be empty")
					}
					if _, exists := cm.Profiles[input]; exists {
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
			profile := interactiveProfileInput(nil)
			cm.Profiles[profileName] = profile
			cm.save()
			fmt.Printf("Profile '%s' added successfully!\n", profileName)
		},
	}

	editCmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			var profileNames []string
			for name := range cm.Profiles {
				profileNames = append(profileNames, name)
			}
			prompt := promptui.Select{
				Label: "Select profile to edit",
				Items: profileNames,
			}
			_, selectedProfile, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			existingProfile := cm.Profiles[selectedProfile]
			updatedProfile := interactiveProfileInput(&existingProfile)
			cm.Profiles[selectedProfile] = updatedProfile
			cm.save()
			fmt.Printf("Profile '%s' updated successfully!\n", selectedProfile)
		},
	}

	removeCmd := &cobra.Command{
		Use:   "rm",
		Short: "Remove a Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			var profileNames []string
			for name := range cm.Profiles {
				profileNames = append(profileNames, name)
			}
			prompt := promptui.Select{
				Label: "Select profile to remove",
				Items: profileNames,
			}
			_, selectedProfile, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			confirmPrompt := promptui.Prompt{
				Label:     fmt.Sprintf("Are you sure you want to remove profile '%s'", selectedProfile),
				IsConfirm: true,
			}
			if _, err := confirmPrompt.Run(); err != nil {
				fmt.Println("Removal cancelled.")
				return
			}
			delete(cm.Profiles, selectedProfile)
			cm.save()
			fmt.Printf("Profile '%s' removed successfully!\n", selectedProfile)
		},
	}

	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a specific Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			var profileNames []string
			for name := range cm.Profiles {
				profileNames = append(profileNames, name)
			}
			prompt := promptui.Select{
				Label: "Select profile to apply",
				Items: profileNames,
			}
			_, selectedProfile, err := prompt.Run()
			if err != nil {
				fmt.Println("Cancelled.")
				return
			}
			profile := cm.Profiles[selectedProfile]
			if _, err := applyProfileInScope(git, "", "", profile, true); err != nil {
				fmt.Printf("Error applying profile: %v\n", err)
				return
			}
			fmt.Printf("Profile '%s' applied successfully!\n", selectedProfile)
		},
	}

	var autoForce bool
	var autoSilent bool
	autoCmd := &cobra.Command{
		Use:   "auto",
		Short: "Automatically apply profile from .gitprofilerc or directory rules",
		Run: func(cmd *cobra.Command, args []string) {
			resolver := AutoResolver{
				GetRepoRoot: func() (string, bool, error) { return findRepoRoot(git) },
				GetHomeDir:  os.UserHomeDir,
				ReadFile:    os.ReadFile,
				FileExists:  func(path string) bool { _, err := os.Stat(path); return err == nil },
				DirectoryRules: appCfg.DirectoryRules,
				GetCurrentDir:  os.Getwd,
			}
			res, err := resolver.Resolve()
			if err != nil {
				if autoSilent {
					return
				}
				fmt.Println("Auto apply failed:", err)
				os.Exit(1)
			}
			profile, ok := cm.Profiles[res.ProfileKey]
			if !ok {
				if autoSilent {
					return
				}
				fmt.Printf("Auto apply failed: profile '%s' not found. Available profiles:\n", res.ProfileKey)
				for k := range cm.Profiles {
					fmt.Printf("- %s\n", k)
				}
				os.Exit(1)
			}
			changed, err := applyProfileInScope(git, res.WorkDir, res.ScopeFlag, profile, autoForce)
			if err != nil {
				fmt.Println("Auto apply failed:", err)
				os.Exit(1)
			}
			if !autoSilent {
				if changed {
					fmt.Printf("Applied profile '%s' from %s (%s).\n", res.ProfileKey, res.RCPath, strings.TrimPrefix(res.ScopeFlag, "--"))
				} else {
					fmt.Printf("No changes: %s config already sets user.name/user.email (use --force to overwrite).\n", strings.TrimPrefix(res.ScopeFlag, "--"))
				}
			}
		},
	}
	autoCmd.Flags().BoolVarP(&autoForce, "force", "f", false, "Overwrite existing user.name/user.email")
	autoCmd.Flags().BoolVar(&autoSilent, "silent", false, "Exit 0 without output when no profile matches (for shell hooks)")

	exportCmd := &cobra.Command{
		Use:   "export [output-file]",
		Short: "Export Git profiles to a JSON file",
		Run: func(cmd *cobra.Command, args []string) {
			var outputPath string
			if len(args) > 0 {
				outputPath = args[0]
			}
			if err := cm.Export(outputPath); err != nil {
				fmt.Println("Export failed:", err)
				os.Exit(1)
			}
		},
	}

	importCmd := &cobra.Command{
		Use:   "import <input-file>",
		Short: "Import Git profiles from a JSON file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := cm.Import(args[0]); err != nil {
				fmt.Println("Import failed:", err)
				os.Exit(1)
			}
		},
	}

	profileCmd.AddCommand(listCmd, addCmd, editCmd, removeCmd, applyCmd, autoCmd, exportCmd, importCmd)
	return profileCmd
}
