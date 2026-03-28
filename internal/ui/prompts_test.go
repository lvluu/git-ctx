package ui

import (
	"testing"
)

func TestProfileListItemsSearchFilter(t *testing.T) {
	items := ProfileListItems{
		{Name: "work", DisplayName: "work", Email: "levi@company.com", IsActive: false},
		{Name: "personal", DisplayName: "personal", Email: "levi@gmail.com", IsActive: true},
		{Name: "work-laptop", DisplayName: "work-laptop", Email: "levi@corp.com", IsActive: false},
	}

	tests := []struct {
		name  string
		query string
		idx   int
		want  bool
	}{
		{"empty query matches all", "", 0, true},
		{"empty query matches second", "", 1, true},
		{"matches name prefix", "work", 0, true},
		{"matches name prefix false on non-match", "work", 1, false},
		{"matches email substring", "gmail", 1, true},
		{"matches email substring false on non-match", "gmail", 0, false},
		{"case insensitive name", "WORK", 0, true},
		{"case insensitive email", "COMPANY", 0, true},
		{"partial name match", "laptop", 2, true},
		{"no match", "xyz", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := items.searchFilter(tt.query, tt.idx)
			if got != tt.want {
				t.Errorf("searchFilter(%q, %d) = %v, want %v", tt.query, tt.idx, got, tt.want)
			}
		})
	}
}

func TestInteractiveProfilePickerEmpty(t *testing.T) {
	_, ok, err := InteractiveProfilePicker(nil, "", "")
	if err == nil {
		t.Error("expected error for nil items, got nil")
	}
	if ok {
		t.Error("expected ok=false for nil items")
	}

	_, ok, err = InteractiveProfilePicker([]ProfileListItem{}, "", "")
	if err == nil {
		t.Error("expected error for empty items, got nil")
	}
	if ok {
		t.Error("expected ok=false for empty items")
	}
}
