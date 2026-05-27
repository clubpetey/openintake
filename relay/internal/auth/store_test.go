package auth_test

import (
	"testing"

	"intake/internal/auth"
)

func TestStore_IssueAndValidate(t *testing.T) {
	s := auth.NewStore()
	id := s.Issue()
	if id == "" {
		t.Fatal("Issue() returned empty string")
	}
	if !s.Validate(id) {
		t.Errorf("Validate(%q) = false; want true", id)
	}
}

func TestStore_UnknownIDFails(t *testing.T) {
	s := auth.NewStore()
	if s.Validate("not-a-real-session") {
		t.Error("Validate(unknown) = true; want false")
	}
}

func TestStore_IssueIsUnique(t *testing.T) {
	s := auth.NewStore()
	a := s.Issue()
	b := s.Issue()
	if a == b {
		t.Errorf("Issue() returned identical IDs: %q", a)
	}
}
