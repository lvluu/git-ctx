// Package profile handles git profile management.
package profile

import (
	"encoding/json"
	"errors"
	"os"
)

// Profile represents a Git profile with name, email, optional signing key,
// optional template inheritance, and optional last-used timestamp.
type Profile struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Extends string `json:"extends,omitempty"`
	Signing struct {
		Key string `json:"key,omitempty"`
	} `json:"signing,omitempty"`
	// LastUsed records when this profile was last applied.
	// Empty if never applied.
	LastUsed string `json:"lastUsed,omitempty"`
}

// Templates is a map of template name -> Profile.
// Templates are partial profiles that can be inherited by other profiles.
type Templates map[string]Profile

// ProfilesStore is the top-level JSON document for the profiles file.
// It supports both the new format (with templates) and legacy format
// (flat map[string]Profile) via backward-compatible unmarshaling.
type ProfilesStore struct {
	Profiles  map[string]Profile `json:"profiles,omitempty"`
	Templates Templates           `json:"templates,omitempty"`
}

// ErrCircularExtends is returned when a profile's template chain contains a cycle.
var ErrCircularExtends = errors.New("circular template inheritance")

// ErrTemplateNotFound is returned when a profile extends a non-existent template.
var ErrTemplateNotFound = errors.New("template not found")

// ResolveProfile resolves a profile against a Templates map.
// It applies templates recursively (A extends B extends C).
// Explicit profile fields override template fields.
// The returned Profile has Extends cleared (not part of the effective profile).
func ResolveProfile(profile Profile, templates Templates) (Profile, error) {
	if profile.Extends == "" {
		result := cloneProfile(profile)
		result.Extends = ""
		return result, nil
	}

	// Walk template chain from leaf to root, collecting templates in order.
	chain := []string{}
	visited := make(map[string]bool)
	current := profile.Extends

	for current != "" {
		if visited[current] {
			return Profile{}, ErrCircularExtends
		}
		visited[current] = true
		chain = append(chain, current)

		tmpl, ok := templates[current]
		if !ok {
			return Profile{}, ErrTemplateNotFound
		}
		current = tmpl.Extends
	}

	// Apply templates in reverse chain order (root→leaf). Because mergeProfile
	// overwrites dst fields with non-empty src values, applying root last means
	// it correctly overrides more-specific (leaf) values when specified.
	result := Profile{}
	for i := len(chain) - 1; i >= 0; i-- {
		tmpl := templates[chain[i]]
		mergeProfile(&result, tmpl)
	}
	mergeProfile(&result, profile)
	result.Extends = ""
	return result, nil
}

// mergeProfile merges non-empty fields from src into dst.
// dst is mutated; src is unchanged.
func mergeProfile(dst *Profile, src Profile) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Email != "" {
		dst.Email = src.Email
	}
	if src.Signing.Key != "" {
		dst.Signing.Key = src.Signing.Key
	}
}

// cloneProfile returns a shallow clone of the profile.
func cloneProfile(p Profile) Profile {
	c := p
	return c
}

// Manager handles loading, saving, and managing profiles.
type Manager struct {
	ConfigPath string
	Profiles   map[string]Profile
	Templates  Templates
}

// NewManager creates a new profile manager.
func NewManager(configPath string) *Manager {
	m := &Manager{
		ConfigPath: configPath,
		Profiles:   make(map[string]Profile),
		Templates:  make(Templates),
	}
	if err := m.load(); err != nil {
		// Log but continue with empty profiles - allows recovery
		m.Profiles = make(map[string]Profile)
		m.Templates = make(Templates)
	}
	return m
}

func (m *Manager) load() error {
	if _, err := os.Stat(m.ConfigPath); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}

	// Try new ProfilesStore format first.
	var store ProfilesStore
	if err := json.Unmarshal(data, &store); err == nil && (len(store.Profiles) > 0 || len(store.Templates) > 0) {
		if store.Profiles == nil {
			store.Profiles = make(map[string]Profile)
		}
		if store.Templates == nil {
			store.Templates = make(Templates)
		}
		m.Profiles = store.Profiles
		m.Templates = store.Templates
		return nil
	}

	// Fall back to legacy flat map for backward compatibility.
	m.Templates = make(Templates)
	if err := json.Unmarshal(data, &m.Profiles); err != nil {
		return err
	}
	return nil
}

func (m *Manager) save() error {
	store := ProfilesStore{
		Profiles:  m.Profiles,
		Templates: m.Templates,
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(m.ConfigPath, data, 0644); err != nil {
		return err
	}
	return nil
}

// Save persists the current profiles to disk.
func (m *Manager) Save() error {
	return m.save()
}

// Get returns the resolved (fully expanded) profile with template inheritance applied.
func (m *Manager) Get(name string) (Profile, bool, error) {
	raw, ok := m.Profiles[name]
	if !ok {
		return Profile{}, false, nil
	}
	resolved, err := ResolveProfile(raw, m.Templates)
	if err != nil {
		return Profile{}, false, err
	}
	return resolved, true, nil
}

// GetRaw returns the raw (unresolved) profile and the name of its template (or "").
func (m *Manager) GetRaw(name string) (Profile, bool) {
	raw, ok := m.Profiles[name]
	if !ok {
		return Profile{}, false
	}
	return raw, true
}

// Templates returns the current templates map.
func (m *Manager) TemplatesMap() Templates {
	return m.Templates
}

// AddTemplate adds or replaces a template.
func (m *Manager) AddTemplate(name string, tmpl Profile) {
	if m.Templates == nil {
		m.Templates = make(Templates)
	}
	m.Templates[name] = tmpl
}

// RemoveTemplate removes a template. Returns true if it existed.
func (m *Manager) RemoveTemplate(name string) bool {
	if m.Templates == nil {
		return false
	}
	_, ok := m.Templates[name]
	if ok {
		delete(m.Templates, name)
	}
	return ok
}

// DiffProfiles compares two resolved profiles and returns the delta.
// The map key is the git config key name (e.g. "user.name").
// The [2]string is [oldValue, newValue]; empty string means the key was absent.
func DiffProfiles(a, b Profile) map[string][2]string {
	delta := make(map[string][2]string)
	configKeys := []struct {
		key   string
		getVal func(p Profile) string
	}{
		{"user.name", func(p Profile) string { return p.Name }},
		{"user.email", func(p Profile) string { return p.Email }},
		{"user.signingkey", func(p Profile) string { return p.Signing.Key }},
	}
	for _, ck := range configKeys {
		oldVal := ck.getVal(a)
		newVal := ck.getVal(b)
		if oldVal != newVal {
			delta[ck.key] = [2]string{oldVal, newVal}
		}
	}
	return delta
}
