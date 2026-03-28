package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// gpgCmd returns an *exec.Cmd for the gpg binary. The binary name is not
// logged or included in any error messages to prevent keyfile path leakage.
func gpgCmd(args ...string) *exec.Cmd {
	return exec.Command("gpg", args...)
}

// FindRepoRoot returns the git repository root directory.
// Returns (root, true, nil) if in a git repo, ("", false, nil) if not, or error on failure.
func FindRepoRoot(git Runner) (root string, inRepo bool, err error) {
	out, err := git.Output("", "rev-parse", "--show-toplevel")
	if err != nil {
		if err == ErrNotGitRepo {
			return "", false, nil
		}
		return "", false, err
	}
	root = strings.TrimSpace(string(out))
	if root == "" {
		return "", false, nil
	}
	return root, true, nil
}

// ConfigGet retrieves a git config value.
// Returns (value, true, nil) if set, ("", false, nil) if unset, or error on failure.
func ConfigGet(git Runner, dir string, scopeFlag string, key string) (value string, isSet bool, err error) {
	args := []string{"config"}
	if scopeFlag != "" {
		args = append(args, scopeFlag)
	}
	args = append(args, "--get", key)
	out, err := git.Output(dir, args...)
	if err != nil {
		if err == ErrGitConfigKeyNotFound {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

// ConfigSet sets a git config value.
func ConfigSet(git Runner, dir string, scopeFlag string, key, value string) error {
	if value == "" {
		return fmt.Errorf("refusing to set %s to empty", key)
	}
	args := []string{"config"}
	if scopeFlag != "" {
		args = append(args, scopeFlag)
	}
	args = append(args, key, value)
	return git.Run(dir, args...)
}

// ApplyProfile applies a profile's name, email, and signing key to git config in the specified scope.
// If force is false, it won't overwrite existing values.
// Returns true if changes were made.
func ApplyProfile(git Runner, dir string, scopeFlag string, name, email, signingKey string, force bool) (changed bool, err error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" {
		return false, fmt.Errorf("profile name and email must both be non-empty")
	}
	items := []struct {
		key   string
		value string
	}{
		{key: "user.name", value: name},
		{key: "user.email", value: email},
	}

	for _, item := range items {
		if !force {
			_, isSet, err := ConfigGet(git, dir, scopeFlag, item.key)
			if err != nil {
				return changed, err
			}
			if isSet {
				continue
			}
		}
		if err := ConfigSet(git, dir, scopeFlag, item.key, item.value); err != nil {
			return changed, err
		}
		changed = true
	}

	// Handle signing key
	if signingKey != "" {
		if err := ConfigSet(git, dir, scopeFlag, "user.signingkey", signingKey); err != nil {
			return changed, err
		}
		if err := ConfigSet(git, dir, scopeFlag, "commit.gpgsign", "true"); err != nil {
			return changed, err
		}
		changed = true
	}

	return changed, nil
}

// ProfileDiff represents a single change to a git config entry.
type ProfileDiff struct {
	Key       string // git config key, e.g. "user.name"
	Action    string // "set", "delete", "skip" (no change needed)
	OldValue  string // current value in git config, "" if unset
	NewValue  string // value from profile, "" if deleting
}

// DiffProfile compares a profile against current git config and returns the diff.
// It makes no changes to git config.
func DiffProfile(git Runner, dir string, scopeFlag string, name, email, signingKey string) ([]ProfileDiff, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" {
		return nil, fmt.Errorf("profile name and email must both be non-empty")
	}

	var diffs []ProfileDiff

	addIfChanged := func(key, newVal string) {
		curVal, isSet, err := ConfigGet(git, dir, scopeFlag, key)
		if err != nil {
			return
		}
		if !isSet {
			diffs = append(diffs, ProfileDiff{Key: key, Action: "set", OldValue: "", NewValue: newVal})
		} else if curVal != newVal {
			diffs = append(diffs, ProfileDiff{Key: key, Action: "set", OldValue: curVal, NewValue: newVal})
		}
	}

	addIfChanged("user.name", name)
	addIfChanged("user.email", email)

	if signingKey != "" {
		// Signing key: profile wants to set it
		curKey, keySet, _ := ConfigGet(git, dir, scopeFlag, "user.signingkey")
		if !keySet {
			diffs = append(diffs, ProfileDiff{Key: "user.signingkey", Action: "set", OldValue: "", NewValue: signingKey})
		} else if curKey != signingKey {
			diffs = append(diffs, ProfileDiff{Key: "user.signingkey", Action: "set", OldValue: curKey, NewValue: signingKey})
		}

		curGPG, gpgSet, _ := ConfigGet(git, dir, scopeFlag, "commit.gpgsign")
		if !gpgSet {
			diffs = append(diffs, ProfileDiff{Key: "commit.gpgsign", Action: "set", OldValue: "", NewValue: "true"})
		} else if curGPG != "true" {
			diffs = append(diffs, ProfileDiff{Key: "commit.gpgsign", Action: "set", OldValue: curGPG, NewValue: "true"})
		}
	}

	return diffs, nil
}

// FormatDryRun formats diffs for --dry-run output.
func FormatDryRun(diffs []ProfileDiff) string {
	var lines []string
	for _, d := range diffs {
		if d.Action == "set" {
			if d.OldValue == "" {
				lines = append(lines, fmt.Sprintf("[DRY RUN] Would set %s %q (currently unset)", d.Key, d.NewValue))
			} else {
				lines = append(lines, fmt.Sprintf("[DRY RUN] Would set %s %q (currently %q)", d.Key, d.NewValue, d.OldValue))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// FormatDiff formats diffs as a git-style diff.
func FormatDiff(diffs []ProfileDiff) string {
	var lines []string
	for _, d := range diffs {
		if d.Action == "set" {
			if d.NewValue == "" {
				lines = append(lines, fmt.Sprintf("-%s = %s", d.Key, d.OldValue))
				lines = append(lines, fmt.Sprintf("+%s", d.Key))
			} else {
				if d.OldValue != "" {
					lines = append(lines, fmt.Sprintf("-%s = %s", d.Key, d.OldValue))
				}
				lines = append(lines, fmt.Sprintf("+%s = %s", d.Key, d.NewValue))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// GetActiveProfile retrieves the currently active Git profile from config.
// Returns (name, email, nil) on success.
func GetActiveProfile(git Runner) (string, string, error) {
	name, nameSet, err := ConfigGet(git, "", "", "user.name")
	if err != nil {
		return "", "", err
	}
	if !nameSet {
		return "", "", nil
	}

	email, emailSet, err := ConfigGet(git, "", "", "user.email")
	if err != nil {
		return "", "", err
	}
	if !emailSet {
		return name, "", nil
	}

	return name, email, nil
}
