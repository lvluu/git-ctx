package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// gpgCmd returns an *exec.Cmd for the gpg binary. The binary name is not
// logged or included in any error messages to prevent keyfile path leakage.
func gpgCmd(args ...string) *exec.Cmd {
	return exec.Command("gpg", args...)
}

// GPGRunner runs GPG commands. It allows tests to inject fake implementations
// without spawning a real GPG binary.
type GPGRunner interface {
	Run(args ...string) error
	Output(args ...string) ([]byte, error)
}

// gpgRealRunner implements GPGRunner using the real gpg binary.
type gpgRealRunner struct{}

// NewGPGRunner returns a GPGRunner backed by the real gpg binary.
func NewGPGRunner() GPGRunner { return &gpgRealRunner{} }

func (g *gpgRealRunner) Run(args ...string) error {
	return gpgCmd(args...).Run()
}
func (g *gpgRealRunner) Output(args ...string) ([]byte, error) {
	return gpgCmd(args...).Output()
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// warnIfWorldReadable checks whether the given file is readable by other users
// and prints a warning to stderr if so. The check is best-effort.
func warnIfWorldReadable(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	// Check world-readable bit (mode & 007)
	if info.Mode().Perm()&007 != 0 {
		fmt.Fprintf(os.Stderr, "warning: GPG keyfile %q is world-readable; consider 'chmod 600 %q'\n", path, path)
	}
}

// ImportGPGKey imports a GPG key from keyfile. The keyfile path is logged;
// its contents are never logged.
func ImportGPGKey(keyfile string, g GPGRunner) error {
	absPath := expandPath(keyfile)
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("GPG keyfile not found: %s", absPath)
	}
	warnIfWorldReadable(absPath)

	// GPG import is idempotent; ignore exit errors so a pre-existing import succeeds.
	_ = g.Run("--import", absPath)
	return nil
}

// ExportGPGKey deletes the secret key identified by fingerprint from the GPG keyring.
func ExportGPGKey(fingerprint string, g GPGRunner) error {
	return g.Run("--batch", "--yes", "--delete-secret-keys", fingerprint)
}

// ValidateGPGKey checks whether the secret key identified by fingerprint is present
// in the local GPG keyring.
func ValidateGPGKey(fingerprint string, g GPGRunner) (bool, error) {
	out, err := g.Output("--batch", "--list-secret-keys", fingerprint)
	if err != nil {
		return false, nil
	}
	// Match the primary key fingerprint line or a subkey fingerprint line.
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "fpr") || strings.HasPrefix(trimmed, "fsb") {
			if strings.Contains(trimmed, fingerprint) {
				return true, nil
			}
		}
	}
	return false, nil
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
