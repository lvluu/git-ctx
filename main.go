package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Profile represents a Git profile with name, email, and optional additional config
type Profile struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Signing struct {
		Key string `json:"key,omitempty"`
	} `json:"signing,omitempty"`
}

// ConfigManager handles loading and saving profiles
type ConfigManager struct {
	ConfigPath string
	Profiles   map[string]Profile
}

// NewConfigManager creates a new config manager using the given profiles file path.
func NewConfigManager(configPath string) *ConfigManager {
	cm := &ConfigManager{
		ConfigPath: configPath,
		Profiles:   make(map[string]Profile),
	}
	cm.load()
	return cm
}

// load reads existing profiles from config file
func (cm *ConfigManager) load() {
	if _, err := os.Stat(cm.ConfigPath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(cm.ConfigPath)
	if err != nil {
		log.Fatal(err)
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &cm.Profiles); err != nil {
			log.Fatal(err)
		}
	}
}

// save writes profiles to config file
func (cm *ConfigManager) save() {
	data, err := json.MarshalIndent(cm.Profiles, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(cm.ConfigPath, data, 0644); err != nil {
		log.Fatal(err)
	}
}

// interactiveProfileInput prompts user for profile details
func interactiveProfileInput(existing *Profile) Profile {
	reader := bufio.NewReader(os.Stdin)
	profile := Profile{}

	// Name input
	if existing != nil && existing.Name != "" {
		fmt.Printf("\nEnter name [current: %s, press Enter to keep]: ", existing.Name)
	} else {
		fmt.Print("Enter name: ")
	}
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" && existing != nil {
		profile.Name = existing.Name
	} else {
		profile.Name = name
	}

	// Email input
	if existing != nil && existing.Email != "" {
		fmt.Printf("Enter email [current: %s, press Enter to keep]: ", existing.Email)
	} else {
		fmt.Print("Enter email: ")
	}
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)
	if email == "" && existing != nil {
		profile.Email = existing.Email
	} else {
		profile.Email = email
	}

	// Optional signing key
	fmt.Print("Enter signing key (optional, press Enter to skip): ")
	signingKey, _ := reader.ReadString('\n')
	signingKey = strings.TrimSpace(signingKey)
	if signingKey != "" {
		profile.Signing.Key = signingKey
	} else if existing != nil {
		profile.Signing.Key = existing.Signing.Key
	}

	return profile
}

// getActiveProfile retrieves the currently active Git profile from the global Git config
func getActiveProfile() (string, string, error) {
	nameCmd := exec.Command("git", "config", "user.name")
	nameOutput, err := nameCmd.Output()
	if err != nil {
		return "", "", err
	}
	name := strings.TrimSpace(string(nameOutput))

	emailCmd := exec.Command("git", "config", "user.email")
	emailOutput, err := emailCmd.Output()
	if err != nil {
		return "", "", err
	}
	email := strings.TrimSpace(string(emailOutput))

	return name, email, nil
}

func main() {
	appCfg, err := loadAppConfig(defaultAppConfigPath())
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	configManager := NewConfigManager(appCfg.ProfilesPath)
	git := ExecGitRunner{}

	var rootCmd = &cobra.Command{
		Use:     "git-ctx",
		Short:   "🦑 Manage git identity and worktree context",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	rootCmd.SetVersionTemplate("🦑 git-ctx\nVersion: {{.Version}}")

	var exportCmd = &cobra.Command{
		Use:   "export [output-file]",
		Short: "Export Git profiles to a JSON file",
		Run: func(cmd *cobra.Command, args []string) {
			var outputPath string
			if len(args) > 0 {
				outputPath = args[0]
			}

			if err := configManager.Export(outputPath); err != nil {
				fmt.Println("Export failed:", err)
				os.Exit(1)
			}
		},
	}

	var importCmd = &cobra.Command{
		Use:   "import <input-file>",
		Short: "Import Git profiles from a JSON file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			inputPath := args[0]

			if err := configManager.Import(inputPath); err != nil {
				fmt.Println("Import failed:", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.AddCommand(exportCmd, importCmd)

	var listCmd = &cobra.Command{
		Use:   "ls",
		Short: "List all saved Git profiles",
		Run: func(cmd *cobra.Command, args []string) {
			if len(configManager.Profiles) == 0 {
				fmt.Println("No profiles found. Use 'git profile add' to create a profile.")
				return
			}

			activeName, activeEmail, err := getActiveProfile()
			if err != nil {
				fmt.Println("Error retrieving active profile:", err)
				return
			}

			for name, profile := range configManager.Profiles {
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

	var addCmd = &cobra.Command{
		Use:   "add",
		Short: "Add a new Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			// Interactive profile name selection
			prompt := promptui.Prompt{
				Label: "Enter profile name",
				Validate: func(input string) error {
					if input == "" {
						return fmt.Errorf("profile name cannot be empty")
					}
					if _, exists := configManager.Profiles[input]; exists {
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

			// Interactive profile details input
			profile := interactiveProfileInput(nil)

			// Save the profile
			configManager.Profiles[profileName] = profile
			configManager.save()

			fmt.Printf("Profile '%s' added successfully!\n", profileName)
		},
	}

	var editCmd = &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			// Select profile to edit
			var profileNames []string
			for name := range configManager.Profiles {
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

			// Existing profile
			existingProfile := configManager.Profiles[selectedProfile]

			// Interactive edit
			updatedProfile := interactiveProfileInput(&existingProfile)

			// Save updated profile
			configManager.Profiles[selectedProfile] = updatedProfile
			configManager.save()

			fmt.Printf("Profile '%s' updated successfully!\n", selectedProfile)
		},
	}

	var removeCmd = &cobra.Command{
		Use:   "rm",
		Short: "Remove a Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			// Select profile to remove
			var profileNames []string
			for name := range configManager.Profiles {
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

			// Confirmation prompt
			confirmPrompt := promptui.Prompt{
				Label:     fmt.Sprintf("Are you sure you want to remove profile '%s'", selectedProfile),
				IsConfirm: true,
			}

			_, confirmErr := confirmPrompt.Run()
			if confirmErr != nil {
				fmt.Println("Removal cancelled.")
				return
			}

			// Remove profile
			delete(configManager.Profiles, selectedProfile)
			configManager.save()

			fmt.Printf("Profile '%s' removed successfully!\n", selectedProfile)
		},
	}

	var applyCmd = &cobra.Command{
		Use:   "apply",
		Short: "Apply a specific Git profile (interactive)",
		Run: func(cmd *cobra.Command, args []string) {
			// Select profile to apply
			var profileNames []string
			for name := range configManager.Profiles {
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

			profile := configManager.Profiles[selectedProfile]

			// Preserve existing behavior: no explicit scope, always overwrite.
			_, applyErr := applyProfileInScope(git, "", "", profile, true)
			if applyErr != nil {
				fmt.Printf("Error applying profile: %v\n", applyErr)
				return
			}

			fmt.Printf("Profile '%s' applied successfully!\n", selectedProfile)
		},
	}

	var autoForce bool
	var autoCmd = &cobra.Command{
		Use:   "auto",
		Short: "Automatically apply profile from .gitprofilerc",
		Run: func(cmd *cobra.Command, args []string) {
			resolver := AutoResolver{
				GetRepoRoot: func() (string, bool, error) { return findRepoRoot(git) },
				GetHomeDir:  os.UserHomeDir,
				ReadFile:    os.ReadFile,
				FileExists: func(path string) bool {
					_, err := os.Stat(path)
					return err == nil
				},
			}

			res, err := resolver.Resolve()
			if err != nil {
				fmt.Println("Auto apply failed:", err)
				os.Exit(1)
			}

			profile, ok := configManager.Profiles[res.ProfileKey]
			if !ok {
				fmt.Printf("Auto apply failed: profile '%s' not found. Available profiles:\n", res.ProfileKey)
				for k := range configManager.Profiles {
					fmt.Printf("- %s\n", k)
				}
				os.Exit(1)
			}

			changed, err := applyProfileInScope(git, res.WorkDir, res.ScopeFlag, profile, autoForce)
			if err != nil {
				fmt.Println("Auto apply failed:", err)
				os.Exit(1)
			}

			if changed {
				fmt.Printf("Applied profile '%s' from %s (%s).\n", res.ProfileKey, res.RCPath, strings.TrimPrefix(res.ScopeFlag, "--"))
			} else {
				fmt.Printf("No changes: %s config already sets user.name/user.email (use --force to overwrite).\n", strings.TrimPrefix(res.ScopeFlag, "--"))
			}
		},
	}
	autoCmd.Flags().BoolVarP(&autoForce, "force", "f", false, "Overwrite existing user.name/user.email")

	var initForce bool
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize git-ctx config file (~/.git-ctx.yaml)",
		Run: func(cmd *cobra.Command, args []string) {
			cfgPath := defaultAppConfigPath()
			if err := initAppConfig(cfgPath, initForce); err != nil {
				fmt.Println("init failed:", err)
				os.Exit(1)
			}
			fmt.Println("Config initialized:", cfgPath)
		},
	}
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config")

	rootCmd.AddCommand(listCmd, addCmd, editCmd, removeCmd, applyCmd, autoCmd, initCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (cm *ConfigManager) Export(outputPath string) error {
	// If no path provided, use default in home directory
	if outputPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		outputPath = filepath.Join(homeDir, "git-profiles-export.json")
	}

	// Ensure the file has .json extension
	if filepath.Ext(outputPath) != ".json" {
		outputPath += ".json"
	}

	// Marshal profiles to JSON
	data, err := json.MarshalIndent(cm.Profiles, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("Profiles exported to: %s\n", outputPath)
	return nil
}

func (cm *ConfigManager) Import(inputPath string) error {
	// Read the input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	// Unmarshal the JSON data
	var importedProfiles map[string]Profile
	if err := json.Unmarshal(data, &importedProfiles); err != nil {
		return err
	}

	// Prompt for import strategy
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

	// Apply import strategy
	switch strategy {
	case "Merge (Add new profiles, keep existing)":
		for name, profile := range importedProfiles {
			if _, exists := cm.Profiles[name]; !exists {
				cm.Profiles[name] = profile
			}
		}
	case "Replace (Overwrite all existing profiles)":
		cm.Profiles = importedProfiles
	}

	// Save the updated profiles
	cm.save()

	fmt.Printf("Profiles imported successfully. Total profiles: %d\n", len(cm.Profiles))
	return nil
}
