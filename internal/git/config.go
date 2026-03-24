package git

import (
	"fmt"
	"strings"
)

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

// ApplyProfile applies a profile's name and email to git config in the specified scope.
// If force is false, it won't overwrite existing values.
// Returns true if changes were made.
func ApplyProfile(git Runner, dir string, scopeFlag string, name, email string, force bool) (changed bool, err error) {
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
	return changed, nil
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
