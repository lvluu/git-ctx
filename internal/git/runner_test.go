package git

import (
	"errors"
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
	if len(args) >= 3 {
		key += " " + args[2]
	}
	if len(args) >= 4 {
		key += " " + args[3]
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
				"config --get user.name": []byte("John Doe"),
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
				"config --get user.name": ErrGitConfigKeyNotFound,
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
				"config user.name Jane Doe":  []byte("John Doe"),
				"config user.email jane@test.com": []byte("john@test.com"),
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

func TestDiffProfile(t *testing.T) {
	t.Run("all values differ", func(t *testing.T) {
		r := &fakeRunner{
			outputs: map[string][]byte{
				"config --get user.name":  []byte("Old Name"),
				"config --get user.email": []byte("old@test.com"),
				"config --get user.signingkey":  []byte("old-key"),
				"config --get commit.gpgsign":   []byte("false"),
			},
		}
		diffs, err := DiffProfile(r, "", "", "New Name", "new@test.com", "ABCDEF")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(diffs) != 4 {
			t.Fatalf("expected 4 diffs, got %d", len(diffs))
		}
		// Check key actions
		keyAction := func(key string) string {
			for _, d := range diffs {
				if d.Key == key {
					return d.Action
				}
			}
			return ""
		}
		if keyAction("user.name") != "set" {
			t.Errorf("expected user.name action 'set', got %q", keyAction("user.name"))
		}
		if keyAction("commit.gpgsign") != "set" {
			t.Errorf("expected commit.gpgsign action 'set', got %q", keyAction("commit.gpgsign"))
		}
	})

	t.Run("no diff when values match", func(t *testing.T) {
		r := &fakeRunner{
			outputs: map[string][]byte{
				"config --get user.name":  []byte("Same Name"),
				"config --get user.email": []byte("same@test.com"),
			},
		}
		diffs, err := DiffProfile(r, "", "", "Same Name", "same@test.com", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(diffs) != 0 {
			t.Errorf("expected 0 diffs when values match, got %d", len(diffs))
		}
	})

	t.Run("unset values show as unset", func(t *testing.T) {
		r := &fakeRunner{
			errs: map[string]error{
				"config --get user.name":  ErrGitConfigKeyNotFound,
				"config --get user.email": ErrGitConfigKeyNotFound,
			},
		}
		diffs, err := DiffProfile(r, "", "", "New", "new@test.com", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(diffs) != 2 {
			t.Fatalf("expected 2 diffs, got %d", len(diffs))
		}
		for _, d := range diffs {
			if d.OldValue != "" {
				t.Errorf("expected OldValue '' for unset key %s, got %q", d.Key, d.OldValue)
			}
		}
	})
}

func TestFormatDryRun(t *testing.T) {
	t.Run("set with old value", func(t *testing.T) {
		diffs := []ProfileDiff{
			{Key: "user.name", Action: "set", OldValue: "Levi Old", NewValue: "Levi"},
		}
		out := FormatDryRun(diffs)
		if out == "" {
			t.Fatal("expected non-empty output")
		}
	})

	t.Run("set with no old value", func(t *testing.T) {
		diffs := []ProfileDiff{
			{Key: "user.name", Action: "set", OldValue: "", NewValue: "Levi"},
		}
		out := FormatDryRun(diffs)
		if out == "" {
			t.Fatal("expected non-empty output")
		}
	})
}

func TestFormatDiff(t *testing.T) {
	diffs := []ProfileDiff{
		{Key: "user.name", Action: "set", OldValue: "Levi Old", NewValue: "Levi"},
		{Key: "user.email", Action: "set", OldValue: "levi@gmail.com", NewValue: "levi@company.com"},
	}
	out := FormatDiff(diffs)
	// Should contain - and + lines
	if !strings.Contains(out, "-user.name = Levi Old") {
		t.Errorf("expected removal line in diff, got: %s", out)
	}
	if !strings.Contains(out, "+user.name = Levi") {
		t.Errorf("expected addition line in diff, got: %s", out)
	}
}
