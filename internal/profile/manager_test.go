package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager(t *testing.T) {
	t.Run("creates new manager with empty profiles", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")
		m := NewManager(path)
		if len(m.Profiles) != 0 {
			t.Errorf("expected 0 profiles, got %d", len(m.Profiles))
		}
	})

	t.Run("saves and loads profiles", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")

		m := NewManager(path)
		m.Profiles["work"] = Profile{Name: "John", Email: "john@work.com"}
		m.Save()

		m2 := NewManager(path)
		if len(m2.Profiles) != 1 {
			t.Errorf("expected 1 profile, got %d", len(m2.Profiles))
		}
		if m2.Profiles["work"].Name != "John" {
			t.Errorf("expected 'John', got %s", m2.Profiles["work"].Name)
		}
	})
}

func TestParseGitProfileRC(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{"plain name", "work", "work"},
		{"profile= key", "profile=personal", "personal"},
		{"profile: key", "profile: personal", "personal"},
		{"comment ignored", "# this is a comment\nwork", "work"},
		{"empty", "", ""},
		{"whitespace only", "   \n  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseGitProfileRC(tt.content)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestResolver(t *testing.T) {
	t.Run("resolves from directory rules", func(t *testing.T) {
		dir := "/home/user/work/project"
		r := Resolver{
			GetRepoRoot:    func() (string, bool, error) { return "", false, nil },
			GetHomeDir:     func() (string, error) { return "/home/user", nil },
			ReadFile:       func(string) ([]byte, error) { return nil, os.ErrNotExist },
			FileExists:     func(string) bool { return false },
			DirectoryRules: []DirectoryRule{{Pattern: "/home/user/work", Profile: "work"}},
			GetCurrentDir:  func() (string, error) { return dir, nil },
		}

		res, err := r.Resolve()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ProfileKey != "work" {
			t.Errorf("expected 'work', got %s", res.ProfileKey)
		}
	})
}
