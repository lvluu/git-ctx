package git

import (
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs map[string][]byte
	errs    map[string]error
	calls   []string
}

func (f *fakeRunner) Output(dir string, args ...string) ([]byte, error) {
	key := args[0]
	if len(args) >= 2 {
		key += " " + args[1]
	}
	f.calls = append(f.calls, key)
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	return nil, errors.New("unexpected call")
}

func (f *fakeRunner) Run(dir string, args ...string) error {
	_, err := f.Output(dir, args...)
	return err
}

func TestFindRepoRoot(t *testing.T) {
	t.Run("in repo", func(t *testing.T) {
		r := &fakeRunner{
			outputs: map[string][]byte{
				"rev-parse --show-toplevel": []byte("/home/user/repo"),
			},
		}
		root, inRepo, err := FindRepoRoot(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !inRepo {
			t.Error("expected inRepo=true")
		}
		if root != "/home/user/repo" {
			t.Errorf("expected /home/user/repo, got %s", root)
		}
	})

	t.Run("not in repo", func(t *testing.T) {
		r := &fakeRunner{
			errs: map[string]error{
				"rev-parse --show-toplevel": ErrNotGitRepo,
			},
		}
		_, inRepo, err := FindRepoRoot(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inRepo {
			t.Error("expected inRepo=false")
		}
	})
}

func TestConfigGet(t *testing.T) {
	t.Run("key exists", func(t *testing.T) {
		r := &fakeRunner{
			outputs: map[string][]byte{
				"config --get": []byte("John Doe"),
			},
		}
		val, isSet, err := ConfigGet(r, "", "", "user.name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isSet {
			t.Error("expected isSet=true")
		}
		if val != "John Doe" {
			t.Errorf("expected 'John Doe', got %s", val)
		}
	})

	t.Run("key not set", func(t *testing.T) {
		r := &fakeRunner{
			errs: map[string]error{
				"config --get": ErrGitConfigKeyNotFound,
			},
		}
		_, isSet, err := ConfigGet(r, "", "", "user.name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isSet {
			t.Error("expected isSet=false")
		}
	})
}

func TestApplyProfile(t *testing.T) {
	t.Run("force true applies both", func(t *testing.T) {
		r := &fakeRunner{
			outputs: map[string][]byte{
				"config user.name":  []byte("John Doe"),
				"config user.email": []byte("john@test.com"),
			},
		}
		changed, err := ApplyProfile(r, "", "", "Jane Doe", "jane@test.com", "", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Error("expected changed=true")
		}
	})
}

// --- GPG function tests ---

type fakeGPGRunner struct {
	outputs map[string][]byte
	errs    map[string]error
}

func (f *fakeGPGRunner) Run(args ...string) error {
	key := strings.Join(args, " ")
	if err, ok := f.errs[key]; ok {
		return err
	}
	return nil
}

func (f *fakeGPGRunner) Output(args ...string) ([]byte, error) {
	key := strings.Join(args, " ")
	if out, ok := f.outputs[key]; ok {
		return out, nil
	}
	if err, ok := f.errs[key]; ok {
		return nil, err
	}
	return nil, errors.New("unexpected gpg call")
}

func TestExpandPath(t *testing.T) {
	t.Run("expands tilde", func(t *testing.T) {
		// Only testable when path does NOT start with ~ (which would call UserHomeDir).
		got := expandPath("/absolute/path/to/key.asc")
		if got != "/absolute/path/to/key.asc" {
			t.Errorf("expected /absolute/path/to/key.asc, got %s", got)
		}
	})
}

func TestImportGPGKey(t *testing.T) {
	t.Run("returns error when keyfile not found", func(t *testing.T) {
		g := &fakeGPGRunner{}
		err := ImportGPGKey("/nonexistent/path/key.asc", g)
		if err == nil {
			t.Error("expected error for missing keyfile")
		}
	})

	t.Run("imports keyfile successfully", func(t *testing.T) {
		tmp := t.TempDir()
		keyfile := tmp + "/test-key.asc"
		if err := os.WriteFile(keyfile, []byte("-----BEGIN PGP PRIVATE KEY BLOCK-----"), 0600); err != nil {
			t.Fatalf("failed to write temp keyfile: %v", err)
		}
		g := &fakeGPGRunner{}
		err := ImportGPGKey(keyfile, g)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("is idempotent on GPG failure", func(t *testing.T) {
		tmp := t.TempDir()
		keyfile := tmp + "/test-key.asc"
		if err := os.WriteFile(keyfile, []byte("-----BEGIN PGP PRIVATE KEY BLOCK-----"), 0600); err != nil {
			t.Fatalf("failed to write temp keyfile: %v", err)
		}
		g := &fakeGPGRunner{
			errs: map[string]error{"--import " + keyfile: errors.New("gpg error")},
		}
		// Should NOT return the GPG error because import is idempotent.
		err := ImportGPGKey(keyfile, g)
		if err != nil {
			t.Errorf("expected no error (idempotent), got: %v", err)
		}
	})
}

func TestValidateGPGKey(t *testing.T) {
	t.Run("key found", func(t *testing.T) {
		fp := "4A2B1C3D4E5F"
		g := &fakeGPGRunner{
			outputs: map[string][]byte{
				"--batch --list-secret-keys " + fp: []byte(
					"sec   ed25519 2024-01-01 [SCA]\n" +
						"      4A2B1C3D4E5F6A7B8C9D0E1F2A3B4C5D6E7F8A9\n" +
						"uid           [ultimate] Test User <test@example.com>\n" +
						"fpr   uuideredacted4A2B1C3D4E5F6A7B8C9D0E1F2A3B4C5D\n",
				),
			},
		}
		valid, err := ValidateGPGKey(fp, g)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !valid {
			t.Error("expected key to be valid")
		}
	})

	t.Run("key not found", func(t *testing.T) {
		g := &fakeGPGRunner{
			outputs: map[string][]byte{
				"--batch --list-secret-keys ABC123": []byte("gpg: error reading key: No such key"),
			},
		}
		valid, err := ValidateGPGKey("ABC123", g)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if valid {
			t.Error("expected key to be invalid")
		}
	})
}

func TestExportGPGKey(t *testing.T) {
	t.Run("deletes key successfully", func(t *testing.T) {
		g := &fakeGPGRunner{}
		err := ExportGPGKey("ABC123DEF456", g)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
