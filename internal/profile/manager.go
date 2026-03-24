// Package profile handles git profile management.
package profile

import (
	"encoding/json"
	"log"
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
	m.load()
	return m
}

func (m *Manager) load() {
	if _, err := os.Stat(m.ConfigPath); os.IsNotExist(err) {
		return
	}
	data, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &m.Profiles); err != nil {
			log.Fatal(err)
		}
	}
}

func (m *Manager) save() {
	data, err := json.MarshalIndent(m.Profiles, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(m.ConfigPath, data, 0644); err != nil {
		log.Fatal(err)
	}
}

// Save persists the current profiles to disk.
func (m *Manager) Save() {
	m.save()
}
