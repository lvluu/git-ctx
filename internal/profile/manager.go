// Package profile handles git profile management.
package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Profile represents a Git profile with name, email, optional signing key,
// and optional template inheritance.
type Profile struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Extends string `json:"extends,omitempty"`
	Signing struct {
		Key string `json:"key,omitempty"`
	} `json:"signing,omitempty"`
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

// GistExporter handles exporting profiles to GitHub Gist.
type GistExporter struct {
	HTTPClient doer
	Token     string
}

// GistImporter handles importing profiles from GitHub Gist.
type GistImporter struct {
	HTTPClient doer
	Token     string
}

// doer abstracts net/http.Client for testing.
type doer interface {
	Do(*http.Request) (*http.Response, error)
}

// gistCreateRequest is the JSON body sent to POST /gists.
type gistCreateRequest struct {
	Description string          `json:"description"`
	Public      bool            `json:"public"`
	Files       map[string]struct {
		Content string `json:"content"`
	} `json:"files"`
}

// gistCreateResponse is the JSON response from POST /gists.
type gistCreateResponse struct {
	ID       string `json:"id"`
	HTMLURL  string `json:"html_url"`
	Files    map[string]struct {
		Content string `json:"content"`
	} `json:"files"`
}

// ExportToGist exports all profiles and templates as a secret Gist.
func (m *Manager) ExportToGist(public bool, httpClient doer, token string) (string, error) {
	ex := GistExporter{HTTPClient: httpClient, Token: token}
	return ex.Export(m.Profiles, m.Templates, public)
}

// Export sends profiles and templates to GitHub Gist API and returns the Gist URL.
func (ex GistExporter) Export(profiles map[string]Profile, templates Templates, public bool) (string, error) {
	store := ProfilesStore{Profiles: profiles, Templates: templates}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return "", err
	}

	body := gistCreateRequest{
		Description: "git-ctx export v1",
		Public:      public,
		Files: map[string]struct {
			Content string `json:"content"`
		}{
			"git-ctx-profiles.json": {Content: string(data)},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/gists", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+ex.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := ex.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("gist API error: status %d", resp.StatusCode)
	}

	var result gistCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.HTMLURL, nil
}

// ImportFromGist imports profiles from a GitHub Gist URL.
// When merge is true, only new profile names are added; when false, all are replaced.
func (m *Manager) ImportFromGist(gistURL string, merge bool, httpClient doer, token string) error {
	im := GistImporter{HTTPClient: httpClient, Token: token}
	profiles, templates, err := im.Import(gistURL)
	if err != nil {
		return err
	}

	if merge {
		for name, p := range profiles {
			if _, exists := m.Profiles[name]; !exists {
				m.Profiles[name] = p
			}
		}
		for name, tmpl := range templates {
			if _, exists := m.Templates[name]; !exists {
				m.Templates[name] = tmpl
			}
		}
	} else {
		m.Profiles = profiles
		m.Templates = templates
	}
	return m.Save()
}

// Import fetches and unmarshals profiles from a GitHub Gist URL.
func (im GistImporter) Import(gistURL string) (map[string]Profile, Templates, error) {
	// Extract Gist ID from URL like https://gist.github.com/<user>/<id> or https://gist.github.com/<id>
	id := extractGistID(gistURL)
	if id == "" {
		return nil, nil, fmt.Errorf("invalid gist URL: %s", gistURL)
	}

	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/gists/"+id, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+im.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := im.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("gist API error: status %d", resp.StatusCode)
	}

	// The GET /gists/:id response has a "files" map keyed by filename.
	var gistResp struct {
		Files map[string]struct {
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gistResp); err != nil {
		return nil, nil, err
	}

	fileData, ok := gistResp.Files["git-ctx-profiles.json"]
	if !ok {
		return nil, nil, fmt.Errorf("gist does not contain git-ctx-profiles.json")
	}

	var store ProfilesStore
	if err := json.Unmarshal([]byte(fileData.Content), &store); err != nil {
		return nil, nil, err
	}

	if store.Profiles == nil {
		store.Profiles = make(map[string]Profile)
	}
	if store.Templates == nil {
		store.Templates = make(Templates)
	}
	return store.Profiles, store.Templates, nil
}

// extractGistID pulls the Gist ID segment from various GitHub Gist URL formats.
func extractGistID(url string) string {
	// https://gist.github.com/<owner>/<id>
	// https://gist.github.com/<id>
	// <id> (raw ID)
	u := strings.TrimPrefix(url, "https://gist.github.com/")
	u = strings.TrimPrefix(u, "http://gist.github.com/")
	u = strings.TrimPrefix(u, "/")
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, ".json")
	if u == "" {
		return ""
	}
	parts := strings.Split(u, "/")
	return parts[len(parts)-1]
}
