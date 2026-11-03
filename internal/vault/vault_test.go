package vault

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func exercise(t *testing.T, store Store, name string) {
	t.Helper()
	if _, err := store.Get(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing secret error = %v", err)
	}
	if err := store.Set(name, "first value"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(name, "second \u00e9 value"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(name)
	if err != nil || got != "second \u00e9 value" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	names, err := store.Names()
	if err != nil || !contains(names, name) {
		t.Fatalf("Names = %v, %v", names, err)
	}
	if err := store.Delete(name); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func TestMemoryStore(t *testing.T) {
	exercise(t, NewMemory(), "demo")
}

func TestValidation(t *testing.T) {
	store := NewMemory()
	for _, name := range []string{"", "-x", "a b", "a/b", strings.Repeat("a", 65)} {
		if err := store.Set(name, "v"); err == nil {
			t.Errorf("name %q was accepted", name)
		}
	}
	if err := store.Set("ok", ""); err == nil {
		t.Error("empty value accepted")
	}
	if err := store.Set("ok", strings.Repeat("x", MaxValueBytes+1)); err == nil {
		t.Error("oversized value accepted")
	}
}

// Uses a unique disposable entry in the current user's Credential Manager.
func TestCredentialManagerRoundTrip(t *testing.T) {
	name := "tau-test-" + time.Now().Format("150405.000000")
	store := NewCredentialManager()
	t.Cleanup(func() { _ = store.Delete(name) })
	exercise(t, store, name)
}
