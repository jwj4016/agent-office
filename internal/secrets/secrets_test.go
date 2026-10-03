package secrets

import (
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func exercise(t *testing.T, s Store) {
	t.Helper()
	ref := NewRef()
	if _, err := s.Get(ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing secret: want ErrNotFound, got %v", err)
	}
	if err := s.Set(ref, "sk-test"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get(ref); err != nil || v != "sk-test" {
		t.Fatalf("get = %q, %v", v, err)
	}
	if err := s.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ref); err != nil {
		t.Fatalf("second delete should be a no-op: %v", err)
	}
	if err := s.Set("plain-key", "x"); !errors.Is(err, ErrInvalidRef) {
		t.Fatalf("want ErrInvalidRef, got %v", err)
	}
}

func TestOpenUsesKeyringWhenAvailable(t *testing.T) {
	keyring.MockInit()
	s := Open()
	if !s.Persistent() {
		t.Fatal("expected keyring store")
	}
	exercise(t, s)
}

func TestOpenFallsBackToMemory(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	s := Open()
	if s.Persistent() {
		t.Fatal("expected memory fallback")
	}
	exercise(t, s)
}

// TestRealKeychain talks to the actual OS keychain. It only runs when
// AGENT_OFFICE_KEYRING_IT=1 so normal test runs never touch it.
func TestRealKeychain(t *testing.T) {
	if os.Getenv("AGENT_OFFICE_KEYRING_IT") != "1" {
		t.Skip("set AGENT_OFFICE_KEYRING_IT=1 to run against the OS keychain")
	}
	s := Open()
	if !s.Persistent() {
		t.Fatal("OS keychain unavailable")
	}
	exercise(t, s)
}
