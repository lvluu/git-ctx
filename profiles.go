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
)

// Profile represents a Git profile with name, email, and optional additional config.
type Profile struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Signing struct {
		Key string `json:"key,omitempty"`
	} `json:"signing,omitempty"`
}

// ConfigManager handles loading and saving profiles.
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

func (cm *ConfigManager) save() {
	data, err := json.MarshalIndent(cm.Profiles, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(cm.ConfigPath, data, 0644); err != nil {
		log.Fatal(err)
	}
}

// Export writes profiles to a JSON file.
func (cm *ConfigManager) Export(outputPath string) error {
	if outputPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		outputPath = filepath.Join(homeDir, "git-ctx-profiles-export.json")
	}
	if filepath.Ext(outputPath) != ".json" {
		outputPath += ".json"
	}
	data, err := json.MarshalIndent(cm.Profiles, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return err
	}
	fmt.Printf("Profiles exported to: %s\n", outputPath)
	return nil
}

// Import reads profiles from a JSON file (interactive strategy selection).
func (cm *ConfigManager) Import(inputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	var importedProfiles map[string]Profile
	if err := json.Unmarshal(data, &importedProfiles); err != nil {
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
		for name, profile := range importedProfiles {
			if _, exists := cm.Profiles[name]; !exists {
				cm.Profiles[name] = profile
			}
		}
	case "Replace (Overwrite all existing profiles)":
		cm.Profiles = importedProfiles
	}
	cm.save()
	fmt.Printf("Profiles imported successfully. Total profiles: %d\n", len(cm.Profiles))
	return nil
}

// interactiveProfileInput prompts user for profile details.
func interactiveProfileInput(existing *Profile) Profile {
	reader := bufio.NewReader(os.Stdin)
	profile := Profile{}

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

// getActiveProfile retrieves the currently active Git profile from the global Git config.
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
