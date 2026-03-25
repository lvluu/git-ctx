package git

import (
	"errors"
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
