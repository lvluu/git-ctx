package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lvluu/git-ctx/internal/profile"
)

func TestProfileRow_JSON(t *testing.T) {
	t.Run("empty profiles prints null", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")
		m := profile.NewManager(path)

		// Capture stdout
		r, w, _ := os.Pipe()
		old := os.Stdout
		os.Stdout = w

		printProfilesJSON(m, "", "")

		w.Close()
		os.Stdout = old

		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		output := string(buf[:n])

		// nil slice marshals to JSON null; empty slice would be [].
		if output != "null\n" {
			t.Errorf("expected null\\n, got %q", output)
		}
		_ = m // suppress unused
	})

	t.Run("JSON output is valid and sorted", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")
		m := profile.NewManager(path)
		m.Profiles = map[string]profile.Profile{
			"zebra": {Name: "Zebra", Email: "zebra@test.com"},
			"alpha": {Name: "Alpha", Email: "alpha@test.com"},
		}

		r, w, _ := os.Pipe()
		old := os.Stdout
		os.Stdout = w

		// alpha profile is active.
		printProfilesJSON(m, "Alpha", "alpha@test.com")

		w.Close()
		os.Stdout = old

		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		output := string(buf[:n])

		var rows []ProfileRow
		if err := json.Unmarshal([]byte(output), &rows); err != nil {
			t.Fatalf("invalid JSON output: %v\n%s", err, output)
		}

		if len(rows) != 2 {
			t.Fatalf("expected 2 rows, got %d", len(rows))
		}

		// Should be sorted by name.
		if rows[0].Name != "alpha" {
			t.Errorf("expected first row name 'alpha', got %q", rows[0].Name)
		}
		if rows[1].Name != "zebra" {
			t.Errorf("expected second row name 'zebra', got %q", rows[1].Name)
		}

		// Active profile detection (alpha profile is active).
		if rows[0].Active != true {
			t.Errorf("expected alpha to be active")
		}
		if rows[1].Active != false {
			t.Errorf("expected zebra to be inactive")
		}
	})

	t.Run("JSON includes template and lastUsed when set", func(t *testing.T) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "profiles.json")
		m := profile.NewManager(path)
		m.Templates = profile.Templates{"base": {Name: "Base", Email: "base@test.com"}}
		m.Profiles = map[string]profile.Profile{
			"work": {Name: "Work", Email: "work@test.com", Extends: "base", LastUsed: "2026-01-15T10:00:00Z"},
		}

		r, w, _ := os.Pipe()
		old := os.Stdout
		os.Stdout = w

		printProfilesJSON(m, "", "")

		w.Close()
		os.Stdout = old

		buf := make([]byte, 4096)
		n, _ := r.Read(buf)
		output := string(buf[:n])

		var rows []ProfileRow
		if err := json.Unmarshal([]byte(output), &rows); err != nil {
			t.Fatalf("invalid JSON output: %v\n%s", err, output)
		}

		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if rows[0].Template != "base" {
			t.Errorf("expected template 'base', got %q", rows[0].Template)
		}
		if rows[0].LastUsed != "2026-01-15T10:00:00Z" {
			t.Errorf("expected lastUsed '2026-01-15T10:00:00Z', got %q", rows[0].LastUsed)
		}
		if rows[0].SigningKey != "" {
			t.Errorf("expected no signing key, got %q", rows[0].SigningKey)
		}
	})
}

func TestMax(t *testing.T) {
	if max(1, 2) != 2 {
		t.Error("max(1,2) should be 2")
	}
	if max(5, 3) != 5 {
		t.Error("max(5,3) should be 5")
	}
	if max(-1, -1) != -1 {
		t.Error("max(-1,-1) should be -1")
	}
}

func TestSpaces(t *testing.T) {
	if spaces(3) != "   " {
		t.Errorf("spaces(3) = %q, want 3 spaces", spaces(3))
	}
	if spaces(0) != "" {
		t.Errorf("spaces(0) = %q, want empty", spaces(0))
	}
	if spaces(-1) != "" {
		t.Errorf("spaces(-1) = %q, want empty", spaces(-1))
	}
}
