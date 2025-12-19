package main

import (
	"fmt"
	"strings"
)

func findRepoRoot(git GitRunner) (root string, inRepo bool, err error) {
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

func gitConfigGet(git GitRunner, dir string, scopeFlag string, key string) (value string, isSet bool, err error) {
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

func gitConfigSet(git GitRunner, dir string, scopeFlag string, key, value string) error {
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

func applyProfileInScope(git GitRunner, dir string, scopeFlag string, profile Profile, force bool) (changed bool, err error) {
	items := []struct {
		key   string
		value string
	}{
		{key: "user.name", value: profile.Name},
		{key: "user.email", value: profile.Email},
	}

	for _, item := range items {
		if !force {
			_, isSet, err := gitConfigGet(git, dir, scopeFlag, item.key)
			if err != nil {
				return changed, err
			}
			if isSet {
				continue
			}
		}
		if err := gitConfigSet(git, dir, scopeFlag, item.key, item.value); err != nil {
			return changed, err
		}
		changed = true
	}
	return changed, nil
}
