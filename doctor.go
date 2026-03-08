package main

import (
	"fmt"
	"os"
	"os/exec"
)

// DoctorResult is one check performed by the doctor command.
type DoctorResult struct {
	Name   string
	OK     bool
	Detail string
}

// runDoctorChecks validates the git-ctx configuration and environment.
func runDoctorChecks(cfg AppConfig) []DoctorResult {
	var results []DoctorResult

	check := func(name string, ok bool, detail string) {
		results = append(results, DoctorResult{Name: name, OK: ok, Detail: detail})
	}

	// 1. git binary
	_, err := exec.LookPath("git")
	if err == nil {
		check("git binary", true, "found")
	} else {
		check("git binary", false, "git not found in PATH")
	}

	// 2. profiles file (missing is OK — created on first use)
	if _, err := os.Stat(cfg.ProfilesPath); os.IsNotExist(err) {
		check("profiles file", true, fmt.Sprintf("%s not yet created (will be created on first 'profile add')", cfg.ProfilesPath))
	} else {
		check("profiles file", true, cfg.ProfilesPath)
	}

	// 3. worktree mode
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

	// 4. directory rules — referenced profiles must exist (only when profiles file is present)
	if _, err := os.Stat(cfg.ProfilesPath); err == nil {
		cm := &ConfigManager{ConfigPath: cfg.ProfilesPath, Profiles: make(map[string]Profile)}
		cm.load()
		for _, rule := range cfg.DirectoryRules {
			if _, ok := cm.Profiles[rule.Profile]; !ok {
				check(
					fmt.Sprintf("directory rule '%s'", rule.Pattern),
					false,
					fmt.Sprintf("profile '%s' not found in %s", rule.Profile, cfg.ProfilesPath),
				)
			} else {
				check(
					fmt.Sprintf("directory rule '%s'", rule.Pattern),
					true,
					fmt.Sprintf("→ profile '%s'", rule.Profile),
				)
			}
		}
	}

	// 5. shell hook hint
	check("shell hook", true, `add 'eval "$(git ctx shell-init)"' to ~/.bashrc or ~/.zshrc`)

	return results
}

// printDoctorResults writes check results to stdout.
func printDoctorResults(results []DoctorResult) {
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
