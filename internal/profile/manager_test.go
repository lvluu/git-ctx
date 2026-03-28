package profile

import (
	"encoding/json"
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

// --- ResolveProfile tests ---

func TestResolveProfile(t *testing.T) {
	t.Run("applies template fields", func(t *testing.T) {
		tmpl := Profile{Name: "Default Name", Email: "default@example.com"}
		profile := Profile{Name: "Levi", Email: "levi@company.com", Extends: "work-base"}
		templates := Templates{"work-base": tmpl}

		resolved, err := ResolveProfile(profile, templates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Name != "Levi" {
			t.Errorf("expected name 'Levi', got %q", resolved.Name)
		}
		if resolved.Email != "levi@company.com" {
			t.Errorf("expected email 'levi@company.com', got %q", resolved.Email)
		}
		if resolved.Extends != "" {
			t.Errorf("expected Extends to be cleared, got %q", resolved.Extends)
		}
	})

	t.Run("fills in missing fields from template", func(t *testing.T) {
		tmpl := Profile{Name: "Default Name", Email: "default@example.com"}
		profile := Profile{Extends: "work-base"}
		templates := Templates{"work-base": tmpl}

		resolved, err := ResolveProfile(profile, templates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Name != "Default Name" {
			t.Errorf("expected name 'Default Name', got %q", resolved.Name)
		}
		if resolved.Email != "default@example.com" {
			t.Errorf("expected email 'default@example.com', got %q", resolved.Email)
		}
	})

	t.Run("does not mutate original profile", func(t *testing.T) {
		tmpl := Profile{Name: "Default Name", Email: "default@example.com"}
		profile := Profile{Name: "Levi", Email: "levi@company.com", Extends: "work-base"}
		templates := Templates{"work-base": tmpl}

		_, err := ResolveProfile(profile, templates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile.Extends != "work-base" {
			t.Errorf("expected original Extends to be 'work-base', got %q", profile.Extends)
		}
	})
}

func TestResolveProfile_CircularExtends(t *testing.T) {
	// A extends B, B extends A
	templates := Templates{
		"a": Profile{Name: "A", Extends: "b"},
		"b": Profile{Name: "B", Extends: "a"},
	}
	profile := Profile{Extends: "a"}

	_, err := ResolveProfile(profile, templates)
	if err != ErrCircularExtends {
		t.Errorf("expected ErrCircularExtends, got %v", err)
	}

	// Self-reference
	selfTemplates := Templates{"self": Profile{Name: "Me", Extends: "self"}}
	selfProfile := Profile{Extends: "self"}

	_, err = ResolveProfile(selfProfile, selfTemplates)
	if err != ErrCircularExtends {
		t.Errorf("expected ErrCircularExtends for self-reference, got %v", err)
	}
}

func TestResolveProfile_TemplateNotFound(t *testing.T) {
	profile := Profile{Extends: "nonexistent"}
	templates := Templates{}

	_, err := ResolveProfile(profile, templates)
	if err != ErrTemplateNotFound {
		t.Errorf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestResolveProfile_DeepChain(t *testing.T) {
	templates := Templates{
		"base":      Profile{Name: "Base Name", Email: "base@example.com"},
		"intermediate": Profile{Name: "Intermediate", Extends: "base"},
		"leaf":      Profile{Email: "leaf@example.com", Extends: "intermediate"},
	}
	profile := Profile{Name: "Leaf User", Extends: "leaf"}

	resolved, err := ResolveProfile(profile, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Explicit field wins
	if resolved.Name != "Leaf User" {
		t.Errorf("expected name 'Leaf User', got %q", resolved.Name)
	}
	// leaf has email, no template above has email → use leaf's
	if resolved.Email != "leaf@example.com" {
		t.Errorf("expected email 'leaf@example.com', got %q", resolved.Email)
	}
}

func TestResolveProfile_OverridePrecedence(t *testing.T) {
	// profile → a → b → c (c is root); "a" provides neither name nor email
	signingWithKey := struct {
		Key string `json:"key,omitempty"`
	}{Key: "root-key"}
	templates := Templates{
		"c": Profile{Name: "Root", Email: "root@example.com", Signing: signingWithKey},
		"b": Profile{Name: "Middle", Extends: "c"},
		"a": Profile{Extends: "b"}, // no name or email — falls through to b
	}
	profile := Profile{Extends: "a"}

	resolved, err := ResolveProfile(profile, templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// profile and a have no name → b: "Middle"
	if resolved.Name != "Middle" {
		t.Errorf("expected name 'Middle', got %q", resolved.Name)
	}
	// profile and a have no email → b has no email → c: "root@example.com"
	if resolved.Email != "root@example.com" {
		t.Errorf("expected email 'root@example.com', got %q", resolved.Email)
	}
	// signing key only in root, no override
	if resolved.Signing.Key != "root-key" {
		t.Errorf("expected signing key 'root-key', got %q", resolved.Signing.Key)
	}
}

func TestResolveProfile_NoExtends(t *testing.T) {
	t.Run("returns cloned profile with cleared extends", func(t *testing.T) {
		profile := Profile{Name: "Solo", Email: "solo@example.com", Extends: ""}
		templates := Templates{"foo": {Name: "T"}}

		resolved, err := ResolveProfile(profile, templates)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved.Name != "Solo" {
			t.Errorf("expected name 'Solo', got %q", resolved.Name)
		}
		if resolved.Email != "solo@example.com" {
			t.Errorf("expected email 'solo@example.com', got %q", resolved.Email)
		}
		if resolved.Extends != "" {
			t.Errorf("expected Extends '', got %q", resolved.Extends)
		}
	})
}

// --- Manager tests ---

func TestManager_Get(t *testing.T) {
	t.Run("returns resolved profile", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")
		m := NewManager(path)
		m.Templates = Templates{"base": {Name: "Base", Email: "base@example.com"}}
		m.Profiles = map[string]Profile{
			"work": {Name: "Work", Email: "work@example.com", Extends: "base"},
		}

		p, ok, err := m.Get("work")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected profile to be found")
		}
		// explicit field wins
		if p.Name != "Work" {
			t.Errorf("expected name 'Work', got %q", p.Name)
		}
	})

	t.Run("returns false for missing profile", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")
		m := NewManager(path)

		_, ok, err := m.Get("nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected not found")
		}
	})
}

func TestManager_LoadTemplates(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")

	store := ProfilesStore{
		Profiles: map[string]Profile{"work": {Name: "Work", Email: "work@example.com"}},
		Templates: Templates{"base": {Name: "Base", Email: "base@example.com"}},
	}
	data, _ := json.MarshalIndent(store, "", "  ")
	os.WriteFile(path, data, 0644)

	m := NewManager(path)
	if len(m.Templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(m.Templates))
	}
	if _, ok := m.Templates["base"]; !ok {
		t.Error("expected template 'base' to be loaded")
	}
}

func TestManager_BackwardCompat(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")

	// Legacy flat format (no templates key)
	legacy := map[string]Profile{"work": {Name: "Work", Email: "work@example.com"}}
	data, _ := json.MarshalIndent(legacy, "", "  ")
	os.WriteFile(path, data, 0644)

	m := NewManager(path)
	if len(m.Profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(m.Profiles))
	}
	if len(m.Templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(m.Templates))
	}
}

func TestManager_AddTemplate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	m := NewManager(path)

	m.AddTemplate("base", Profile{Name: "Base", Email: "base@example.com"})
	if len(m.Templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(m.Templates))
	}
}

func TestManager_RemoveTemplate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	m := NewManager(path)
	m.Templates = Templates{"base": {Name: "Base"}}

	ok := m.RemoveTemplate("base")
	if !ok {
		t.Error("expected RemoveTemplate to return true")
	}
	if len(m.Templates) != 0 {
		t.Errorf("expected 0 templates, got %d", len(m.Templates))
	}

	ok = m.RemoveTemplate("nonexistent")
	if ok {
		t.Error("expected RemoveTemplate to return false for missing template")
	}
}

// --- DiffProfiles tests ---

func TestDiffProfiles(t *testing.T) {
	tests := []struct {
		name        string
		profileA    Profile
		profileB    Profile
		wantKeys    []string          // keys expected in delta
		wantPair    map[string][2]string // exact pairs expected
	}{
		{
			name:     "identical profiles produce no delta",
			profileA: Profile{Name: "Alice", Email: "alice@example.com"},
			profileB: Profile{Name: "Alice", Email: "alice@example.com"},
			wantKeys: nil,
			wantPair: nil,
		},
		{
			name:     "different name",
			profileA: Profile{Name: "Alice", Email: "alice@example.com"},
			profileB: Profile{Name: "Bob", Email: "alice@example.com"},
			wantKeys: []string{"user.name"},
			wantPair: map[string][2]string{"user.name": {"Alice", "Bob"}},
		},
		{
			name:     "different email",
			profileA: Profile{Name: "Alice", Email: "alice@example.com"},
			profileB: Profile{Name: "Alice", Email: "bob@example.com"},
			wantKeys: []string{"user.email"},
			wantPair: map[string][2]string{"user.email": {"alice@example.com", "bob@example.com"}},
		},
		{
			name:     "different signing key",
			profileA: func() Profile { p := Profile{Name: "Alice", Email: "alice@example.com"}; p.Signing.Key = "AAA"; return p }(),
			profileB: func() Profile { p := Profile{Name: "Alice", Email: "alice@example.com"}; p.Signing.Key = "BBB"; return p }(),
			wantKeys: []string{"user.signingkey"},
			wantPair: map[string][2]string{"user.signingkey": {"AAA", "BBB"}},
		},
		{
			name:     "key absent in profile A, present in profile B",
			profileA: Profile{Name: "Alice", Email: "alice@example.com"},
			profileB: func() Profile { p := Profile{Name: "Alice", Email: "alice@example.com"}; p.Signing.Key = "KEY"; return p }(),
			wantKeys: []string{"user.signingkey"},
			wantPair: map[string][2]string{"user.signingkey": {"", "KEY"}},
		},
		{
			name:     "key absent in profile B, present in profile A",
			profileA: func() Profile { p := Profile{Name: "Alice", Email: "alice@example.com"}; p.Signing.Key = "KEY"; return p }(),
			profileB: Profile{Name: "Alice", Email: "alice@example.com"},
			wantKeys: []string{"user.signingkey"},
			wantPair: map[string][2]string{"user.signingkey": {"KEY", ""}},
		},
		{
			name:     "multiple keys differ",
			profileA: Profile{Name: "Alice", Email: "alice@example.com"},
			profileB: func() Profile { p := Profile{Name: "Bob", Email: "bob@example.com"}; p.Signing.Key = "KEY"; return p }(),
			wantKeys: []string{"user.name", "user.email", "user.signingkey"},
			wantPair: map[string][2]string{
				"user.name":       {"Alice", "Bob"},
				"user.email":      {"alice@example.com", "bob@example.com"},
				"user.signingkey": {"", "KEY"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delta := DiffProfiles(tt.profileA, tt.profileB)
			if len(delta) != len(tt.wantKeys) {
				t.Errorf("expected %d keys in delta, got %d: %v", len(tt.wantKeys), len(delta), delta)
			}
			for _, k := range tt.wantKeys {
				pair, ok := delta[k]
				if !ok {
					t.Errorf("expected key %q in delta, but it was missing", k)
					continue
				}
				wantPair := tt.wantPair[k]
				if pair != wantPair {
					t.Errorf("delta[%q] = %v, want %v", k, pair, wantPair)
				}
			}
		})
	}
}

func TestManager_GetRaw(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "profiles.json")
	m := NewManager(path)
	m.Templates = Templates{"base": {Name: "Base", Email: "base@example.com"}}
	m.Profiles = map[string]Profile{
		"work": {Name: "Work", Email: "work@example.com", Extends: "base"},
	}

	p, ok := m.GetRaw("work")
	if !ok {
		t.Fatal("expected profile to be found")
	}
	if p.Extends != "base" {
		t.Errorf("expected raw extends 'base', got %q", p.Extends)
	}
}
