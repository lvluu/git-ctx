// Package profile handles git profile management.
package profile

import (
	"encoding/json"
	"os"
)

// Profile represents a Git profile with name, email, and optional signing key.
type Profile struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Signing struct {
		Key string `json:"key,omitempty"`
	} `json:"signing,omitempty"`
}

// Manager handles loading, saving, and managing profiles.
type Manager struct {
	ConfigPath string
	Profiles   map[string]Profile
}

// NewManager creates a new profile manager.
func NewManager(configPath string) *Manager {
	m := &Manager{
		ConfigPath: configPath,
		Profiles:   make(map[string]Profile),
	}
	if err := m.load(); err != nil {
		// Log but continue with empty profiles - allows recovery
		m.Profiles = make(map[string]Profile)
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
	if len(data) > 0 {
		if err := json.Unmarshal(data, &m.Profiles); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.Profiles, "", "  ")
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
