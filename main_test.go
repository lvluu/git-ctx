package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeGitRunner struct {
	values map[string]string
	set    map[string]string

	// If set, Output will return this error for rev-parse.
	revParseErr error
}

func (f *fakeGitRunner) Output(dir string, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--show-toplevel" {
		if f.revParseErr != nil {
			return nil, f.revParseErr
		}
		// For tests that want an in-repo behavior, the repo root is passed via value "__repo_root__".
		if root, ok := f.values["__repo_root__"]; ok {
			return []byte(root), nil
		}
		return []byte(""), nil
	}

	// git config [--local|--global] --get <key>
	if len(args) >= 4 && args[0] == "config" && args[len(args)-2] == "--get" {
		key := args[len(args)-1]
		scope := ""
		for _, a := range args {
			if a == "--local" || a == "--global" {
				scope = a
				break
			}
		}
		lookup := scope + ":" + key
		if v, ok := f.values[lookup]; ok {
			return []byte(v), nil
		}
		return nil, ErrGitConfigKeyNotFound
	}

	return nil, errors.New("unexpected git output call: " + strings.Join(args, " "))
}

func (f *fakeGitRunner) Run(dir string, args ...string) error {
	// git config [--local|--global] <key> <value>
	if len(args) >= 4 && args[0] == "config" {
		scope := ""
		idx := 1
		if args[1] == "--local" || args[1] == "--global" {
			scope = args[1]
			idx = 2
		}
		key := args[idx]
		value := args[idx+1]
		if f.set == nil {
			f.set = map[string]string{}
		}
		f.set[scope+":"+key] = value
		return nil
	}
	return errors.New("unexpected git run call: " + strings.Join(args, " "))
}

// TestConfigManager tests the configuration management functionality
func TestConfigManager(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "git-profile-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test config path
	testConfigPath := filepath.Join(tmpDir, ".git-profiles-test.json")

	// Create a config manager with the test path
	cm := &ConfigManager{
		ConfigPath: testConfigPath,
		Profiles:   make(map[string]Profile),
	}

	// Test adding a profile
	testProfile := Profile{
		Name:  "John Doe",
		Email: "john.doe@example.com",
	}
	cm.Profiles["work"] = testProfile
	cm.save()

	// Verify the file was created
	_, err = os.Stat(testConfigPath)
	assert.NoError(t, err)

	// Read the file contents
	data, err := os.ReadFile(testConfigPath)
	assert.NoError(t, err)

	// Verify the contents
	var loadedProfiles map[string]Profile
	err = json.Unmarshal(data, &loadedProfiles)
	assert.NoError(t, err)
	assert.Contains(t, loadedProfiles, "work")
	assert.Equal(t, "John Doe", loadedProfiles["work"].Name)
	assert.Equal(t, "john.doe@example.com", loadedProfiles["work"].Email)
}

// TestProfileValidation tests profile input validation
func TestProfileValidation(t *testing.T) {
	// Test with completely new profile
	newProfile := Profile{
		Name:  "Jane Smith",
		Email: "jane.smith@example.com",
	}
	assert.NotEmpty(t, newProfile.Name)
	assert.NotEmpty(t, newProfile.Email)

	// Test with existing profile and partial update
	existingProfile := Profile{
		Name:  "John Doe",
		Email: "john.doe@example.com",
	}

	// Simulate interactive update with some fields kept
	updatedProfile := Profile{
		Name:  "", // Should keep existing name
		Email: "john.updated@example.com",
	}

	// Merge logic
	if updatedProfile.Name == "" {
		updatedProfile.Name = existingProfile.Name
	}
	if updatedProfile.Email == "" {
		updatedProfile.Email = existingProfile.Email
	}

	assert.Equal(t, "John Doe", updatedProfile.Name)
	assert.Equal(t, "john.updated@example.com", updatedProfile.Email)
}

// TestProfileSerialization tests JSON serialization and deserialization
func TestProfileSerialization(t *testing.T) {
	// Create a profile with all fields
	profile := Profile{
		Name:  "Alice Johnson",
		Email: "alice.johnson@example.com",
	}
	profile.Signing.Key = "1234ABCD"

	// Serialize to JSON
	jsonData, err := json.Marshal(profile)
	assert.NoError(t, err)

	// Deserialize back to Profile
	var decodedProfile Profile
	err = json.Unmarshal(jsonData, &decodedProfile)
	assert.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, "Alice Johnson", decodedProfile.Name)
	assert.Equal(t, "alice.johnson@example.com", decodedProfile.Email)
	assert.Equal(t, "1234ABCD", decodedProfile.Signing.Key)
}

// TestMultipleProfiles tests managing multiple profiles
func TestMultipleProfiles(t *testing.T) {
	// Create a config manager
	cm := &ConfigManager{
		Profiles: make(map[string]Profile),
	}

	// Add multiple profiles
	cm.Profiles["work"] = Profile{
		Name:  "John Doe",
		Email: "john.doe@company.com",
	}
	cm.Profiles["personal"] = Profile{
		Name:  "John Personal",
		Email: "john.personal@gmail.com",
	}

	// Verify number of profiles
	assert.Equal(t, 2, len(cm.Profiles))

	// Verify individual profile details
	workProfile, exists := cm.Profiles["work"]
	assert.True(t, exists)
	assert.Equal(t, "John Doe", workProfile.Name)

	personalProfile, exists := cm.Profiles["personal"]
	assert.True(t, exists)
	assert.Equal(t, "John Personal", personalProfile.Name)
}

// TestProfileRemoval tests removing a profile
func TestProfileRemoval(t *testing.T) {
	// Create a config manager with some profiles
	cm := &ConfigManager{
		Profiles: map[string]Profile{
			"work":     {Name: "John Doe", Email: "john.doe@company.com"},
			"personal": {Name: "John Personal", Email: "john.personal@gmail.com"},
		},
	}

	// Initial count
	assert.Equal(t, 2, len(cm.Profiles))

	// Remove a profile
	delete(cm.Profiles, "work")

	// Verify removal
	assert.Equal(t, 1, len(cm.Profiles))
	_, exists := cm.Profiles["work"]
	assert.False(t, exists)
}

// TestExport tests the Export function
func TestExport(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "git-profile-test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test config path
	testConfigPath := filepath.Join(tmpDir, ".git-profiles-test.json")

	// Create a config manager with the test path
	cm := &ConfigManager{
		ConfigPath: testConfigPath,
		Profiles:   make(map[string]Profile),
	}

	// Add a profile
	cm.Profiles["work"] = Profile{
		Name:  "John Doe",
		Email: "john.doe@example.com",
	}

	// Export profiles
	exportPath := filepath.Join(tmpDir, "exported-profiles.json")
	err = cm.Export(exportPath)
	assert.NoError(t, err)

	// Verify the file was created
	_, err = os.Stat(exportPath)
	assert.NoError(t, err)

	// Read the file contents
	data, err := os.ReadFile(exportPath)
	assert.NoError(t, err)

	// Verify the contents
	var exportedProfiles map[string]Profile
	err = json.Unmarshal(data, &exportedProfiles)
	assert.NoError(t, err)
	assert.Contains(t, exportedProfiles, "work")
	assert.Equal(t, "John Doe", exportedProfiles["work"].Name)
	assert.Equal(t, "john.doe@example.com", exportedProfiles["work"].Email)
}

func TestParseGitProfileRC(t *testing.T) {
	t.Run("first non-comment line", func(t *testing.T) {
		key, err := parseGitProfileRC([]byte("\n# comment\nwork\n"))
		assert.NoError(t, err)
		assert.Equal(t, "work", key)
	})

	t.Run("profile equals", func(t *testing.T) {
		key, err := parseGitProfileRC([]byte("profile=personal\n"))
		assert.NoError(t, err)
		assert.Equal(t, "personal", key)
	})

	t.Run("profile colon", func(t *testing.T) {
		key, err := parseGitProfileRC([]byte("profile:  work\n"))
		assert.NoError(t, err)
		assert.Equal(t, "work", key)
	})

	t.Run("empty file", func(t *testing.T) {
		_, err := parseGitProfileRC([]byte("\n  \n# c\n"))
		assert.Error(t, err)
	})
}

func TestAutoResolver_ProjectRCWinsAndIsLocal(t *testing.T) {
	tmp := t.TempDir()
	projectRC := filepath.Join(tmp, ".gitprofilerc")
	err := os.WriteFile(projectRC, []byte("work\n"), 0644)
	assert.NoError(t, err)

	resolver := AutoResolver{
		GetRepoRoot: func() (string, bool, error) { return tmp, true, nil },
		GetHomeDir:  func() (string, error) { return t.TempDir(), nil },
		ReadFile:    os.ReadFile,
		FileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}

	res, err := resolver.Resolve()
	assert.NoError(t, err)
	assert.Equal(t, "work", res.ProfileKey)
	assert.Equal(t, "--local", res.ScopeFlag)
	assert.Equal(t, tmp, res.WorkDir)
	assert.Equal(t, projectRC, res.RCPath)
}

func TestAutoResolver_FallsBackToHomeRCAndIsGlobal(t *testing.T) {
	home := t.TempDir()
	homeRC := filepath.Join(home, ".gitprofilerc")
	err := os.WriteFile(homeRC, []byte("personal\n"), 0644)
	assert.NoError(t, err)

	resolver := AutoResolver{
		GetRepoRoot: func() (string, bool, error) { return "", false, nil },
		GetHomeDir:  func() (string, error) { return home, nil },
		ReadFile:    os.ReadFile,
		FileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}

	res, err := resolver.Resolve()
	assert.NoError(t, err)
	assert.Equal(t, "personal", res.ProfileKey)
	tassert := assert.New(t)
	tassert.Equal("--global", res.ScopeFlag)
	tassert.Equal("", res.WorkDir)
	tassert.Equal(homeRC, res.RCPath)
}

func TestApplyProfileInScope_RespectsExistingUnlessForced(t *testing.T) {
	profile := Profile{Name: "Jane", Email: "jane@example.com"}
	git := &fakeGitRunner{values: map[string]string{}}

	// Existing local config: name already set, email not set.
	git.values["--local:user.name"] = "Already"

	changed, err := applyProfileInScope(git, "C:/repo", "--local", profile, false)
	assert.NoError(t, err)
	assert.True(t, changed)
	assert.NotContains(t, git.set, "--local:user.name")
	assert.Equal(t, "jane@example.com", git.set["--local:user.email"])

	git2 := &fakeGitRunner{values: map[string]string{"--local:user.name": "Already", "--local:user.email": "already@example.com"}}
	changed2, err := applyProfileInScope(git2, "C:/repo", "--local", profile, false)
	assert.NoError(t, err)
	assert.False(t, changed2)

	git3 := &fakeGitRunner{values: map[string]string{"--local:user.name": "Already", "--local:user.email": "already@example.com"}}
	changed3, err := applyProfileInScope(git3, "C:/repo", "--local", profile, true)
	assert.NoError(t, err)
	assert.True(t, changed3)
	assert.Equal(t, "Jane", git3.set["--local:user.name"])
	assert.Equal(t, "jane@example.com", git3.set["--local:user.email"])
}

// TODO: Test import functionality
