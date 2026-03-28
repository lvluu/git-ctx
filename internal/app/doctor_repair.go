package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/lvluu/git-ctx/internal/config"
	"github.com/lvluu/git-ctx/internal/profile"
)

// Repairable represents a doctor check that can be automatically repaired.
type Repairable struct {
	// Description explains what this repair does.
	Description string
	// Fix performs the repair. dryRun reports what would happen without doing it.
	Fix func(dryRun bool) (RepairResult, error)
}

// RepairResult describes the outcome of a repair attempt.
type RepairResult struct {
	Success     bool
	Summary     string // one-line summary
	BackupPath  string // if a backup was made, path to it
	DryRunHints []string
}

// repoRootsExtractor is used to collect doctorResult details for repair lookups.
func runDoctorRepair(
	results []doctorResult,
	cfg config.AppConfig,
	repoRoot string,
	gitBin string,
	mgr *profile.Manager,
	dryRun bool,
) map[string]RepairResult {
	outcomes := make(map[string]RepairResult)

	for _, r := range results {
		if r.OK {
			continue
		}
		result, err := repairForResult(r, cfg, mgr, dryRun)
		if err != nil {
			outcomes[r.Name] = RepairResult{Success: false, Summary: "error: " + err.Error()}
		} else if result.Success {
			outcomes[r.Name] = result
		}
	}

	return outcomes
}

// repairForResult returns the appropriate repair for a failed doctor result.
func repairForResult(r doctorResult, cfg config.AppConfig, mgr *profile.Manager, dryRun bool) (RepairResult, error) {
	switch r.Name {
	case "shell hook":
		return fixShellHook(dryRun)

	case "profiles file":
		return fixProfilesFile(cfg.ProfilesPath, dryRun)
	}

	// Handle "directory rule '...'" checks — extract profile name from Detail.
	if strings.HasPrefix(r.Name, "directory rule '") {
		profileName := extractProfileName(r.Detail)
		if profileName != "" {
			return fixMissingProfile(profileName, mgr, dryRun)
		}
	}

	return RepairResult{Success: false}, nil
}

// extractProfileName pulls "profile 'X'" from a doctor detail string.
var profileNameRE = regexp.MustCompile(`profile '([^']+)'`)

func extractProfileName(detail string) string {
	m := profileNameRE.FindStringSubmatch(detail)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// ------------------------------------------------------------------
// Shell hook repair
// ------------------------------------------------------------------

// fixShellHook appends the git-ctx init snippet to ~/.bashrc and ~/.zshrc
// if the shell init line is not already present.
func fixShellHook(dryRun bool) (RepairResult, error) {
	snippet := ShellInitScript()
	shellRCs := []string{
		os.ExpandEnv("$HOME/.bashrc"),
		os.ExpandEnv("$HOME/.zshrc"),
	}
	results := []string{}
	backups := []string{}

	for _, rc := range shellRCs {
		fixed, backupPath, err := ensureShellHook(rc, snippet, dryRun)
		if err != nil {
			return RepairResult{}, err
		}
		if fixed {
			if dryRun {
				results = append(results, fmt.Sprintf("Would append to %s", rc))
			} else {
				results = append(results, fmt.Sprintf("Updated %s", rc))
				if backupPath != "" {
					backups = append(backups, backupPath)
				}
			}
		} else {
			results = append(results, fmt.Sprintf("Already present in %s", rc))
		}
	}

	if len(results) == 0 {
		return RepairResult{Success: true, Summary: "No shell init file found"}, nil
	}

	backupPath := ""
	if len(backups) > 0 {
		backupPath = backups[0]
	}
	return RepairResult{
		Success:    true,
		Summary:    strings.Join(results, "; "),
		BackupPath: backupPath,
	}, nil
}

// ensureShellHook appends snippet to rcPath if the eval line is not already present.
// Returns (changed, backupPath, error).
// On dryRun=true, returns (wouldChange, "", nil) without modifying anything.
func ensureShellHook(rcPath, snippet string, dryRun bool) (changed bool, backupPath string, err error) {
	marker := `eval "$(git ctx shell-init)"`
	aliasLine := `alias gc="git-ctx"`

	content, err := os.ReadFile(rcPath)
	if os.IsNotExist(err) {
		if dryRun {
			return true, "", nil
		}
		f, err := os.Create(rcPath)
		if err != nil {
			return false, "", fmt.Errorf("create %s: %w", rcPath, err)
		}
		f.Close()
		if err := os.WriteFile(rcPath, []byte(snippet), 0644); err != nil {
			return false, "", fmt.Errorf("write %s: %w", rcPath, err)
		}
		return true, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("read %s: %w", rcPath, err)
	}

	text := string(content)
	if strings.Contains(text, marker) && strings.Contains(text, aliasLine) {
		return false, "", nil
	}

	if dryRun {
		return true, "", nil
	}

	backup := rcPath + ".git-ctx.bak"
	if err := os.WriteFile(backup, content, 0644); err != nil {
		return false, "", fmt.Errorf("backup %s: %w", backup, err)
	}

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return false, "", fmt.Errorf("open %s: %w", rcPath, err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + snippet + "\n"); err != nil {
		return false, "", fmt.Errorf("append to %s: %w", rcPath, err)
	}

	return true, backup, nil
}

// ------------------------------------------------------------------
// Profiles file repair
// ------------------------------------------------------------------

// fixProfilesFile backs up the existing profiles file and writes an empty
// valid ProfilesStore so the user can start fresh.
func fixProfilesFile(profilesPath string, dryRun bool) (RepairResult, error) {
	_, statErr := os.Stat(profilesPath)
	fileExists := statErr == nil

	if !fileExists {
		if dryRun {
			return RepairResult{
				Success:     true,
				Summary:     fmt.Sprintf("Would create %s", profilesPath),
				DryRunHints: []string{"File does not exist — no backup needed"},
			}, nil
		}
		if err := os.WriteFile(profilesPath, []byte(profilesStoreJSON()), 0644); err != nil {
			return RepairResult{Success: false, Summary: "error: " + err.Error()}, err
		}
		return RepairResult{
			Success: true,
			Summary: fmt.Sprintf("Created %s", profilesPath),
		}, nil
	}

	data, err := os.ReadFile(profilesPath)
	if err != nil {
		return RepairResult{Success: false, Summary: "error: " + err.Error()}, err
	}

	// Try ProfilesStore format.
	var store struct {
		Profiles  map[string]any `json:"profiles"`
		Templates map[string]any `json:"templates"`
	}
	if err := json.Unmarshal(data, &store); err == nil && (store.Profiles != nil || store.Templates != nil) {
		return RepairResult{
			Success: true,
			Summary: fmt.Sprintf("%s is already valid — no repair needed", profilesPath),
		}, nil
	}

	// Try legacy flat map.
	var legacy map[string]any
	if err := json.Unmarshal(data, &legacy); err == nil {
		return RepairResult{
			Success: true,
			Summary: fmt.Sprintf("%s is valid (legacy format) — no repair needed", profilesPath),
		}, nil
	}

	// File is corrupted.
	if dryRun {
		return RepairResult{
			Success: true,
			Summary: fmt.Sprintf("Would backup and re-initialize %s", profilesPath),
			DryRunHints: []string{
				fmt.Sprintf("Backup: %s.git-ctx.bak", profilesPath),
				"Content: empty profiles store",
			},
		}, nil
	}

	backup := profilesPath + ".git-ctx.bak"
	if err := copyFile(backup, profilesPath); err != nil {
		return RepairResult{Success: false, Summary: "error: " + err.Error()}, err
	}
	if err := os.WriteFile(profilesPath, []byte(profilesStoreJSON()), 0644); err != nil {
		return RepairResult{Success: false, Summary: "error: " + err.Error()}, err
	}
	return RepairResult{
		Success:    true,
		Summary:    fmt.Sprintf("Backed up and re-initialized %s", profilesPath),
		BackupPath: backup,
	}, nil
}

// profilesStoreJSON returns an empty, valid profiles store as JSON.
func profilesStoreJSON() string {
	return "{\n  \"profiles\": {}\n}\n"
}

// ------------------------------------------------------------------
// Missing profile repair
// ------------------------------------------------------------------

// fixMissingProfile creates a stub profile with the given name via the profile manager.
func fixMissingProfile(profileName string, mgr *profile.Manager, dryRun bool) (RepairResult, error) {
	if dryRun {
		return RepairResult{
			Success:     true,
			Summary:     fmt.Sprintf("Would create stub profile '%s'", profileName),
			DryRunHints: []string{"Stub profile: name=<unset>, email=<unset>, signing.key omitted"},
		}, nil
	}

	if mgr == nil {
		return RepairResult{Success: false, Summary: "profile manager not available"}, nil
	}

	if _, exists := mgr.Profiles[profileName]; exists {
		return RepairResult{
			Success: true,
			Summary: fmt.Sprintf("Profile '%s' already exists — no repair needed", profileName),
		}, nil
	}

	mgr.Profiles[profileName] = profile.Profile{Name: "", Email: ""}
	if err := mgr.Save(); err != nil {
		return RepairResult{Success: false, Summary: "error: " + err.Error()}, err
	}
	return RepairResult{
		Success: true,
		Summary: fmt.Sprintf("Created stub profile '%s' in %s", profileName, mgr.ConfigPath),
	}, nil
}

// ------------------------------------------------------------------
// Utilities
// ------------------------------------------------------------------

// copyFile copies src to dst using io.Copy.
func copyFile(dst, src string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}
