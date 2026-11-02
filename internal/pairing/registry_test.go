package pairing

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// reverseSealer is a fake: it is reversible but never equals the plain bytes.
type reverseSealer struct{}

func (reverseSealer) Seal(plain []byte) ([]byte, error) {
	out := make([]byte, len(plain))
	for index, value := range plain {
		out[len(plain)-1-index] = value ^ 0xA5
	}
	return out, nil
}

func (reverseSealer) Open(sealed []byte) ([]byte, error) { return reverseSealer{}.Seal(sealed) }

type fakeClock struct{ now time.Time }

func (clock *fakeClock) Now() time.Time { return clock.now }

func newTestRegistry(t *testing.T, clock *fakeClock) (*Registry, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devices.json")
	registry, err := OpenRegistry(path, reverseSealer{}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return registry, path
}

func TestRegistryStoresNoSecretsAndSurvivesRestart(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	registry, path := newTestRegistry(t, clock)
	device, token, err := registry.Add("Pixel", []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte(token)) {
		t.Fatal("the bearer token must not be stored, only its hash")
	}
	certificate, _ := registry.Certificate(device.ID)
	plainKey, _ := reverseSealer{}.Open(mustSealed(t, registry, device.ID))
	if bytes.Contains(raw, plainKey) || certificate == nil {
		t.Fatal("the private key must be stored sealed")
	}
	reopened, err := OpenRegistry(path, reverseSealer{}, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Authenticate(device.ID, token); err != nil {
		t.Fatalf("a paired device must survive a restart: %v", err)
	}
}

func mustSealed(t *testing.T, registry *Registry, id string) []byte {
	t.Helper()
	return registry.records[id].SealedKey
}

func TestAuthenticateRejectsWrongExpiredAndRevoked(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	registry, _ := newTestRegistry(t, clock)
	device, token, _ := registry.Add("Pixel", nil, time.Hour)
	if _, err := registry.Authenticate(device.ID, "wrong"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong token: %v", err)
	}
	if _, err := registry.Authenticate("nope", token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown id: %v", err)
	}
	other, otherToken, _ := registry.Add("Tablet", nil, time.Hour)
	if _, err := registry.Authenticate(device.ID, otherToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("another device's token must not work")
	}
	if err := registry.Revoke(other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Authenticate(other.ID, otherToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("a revoked device must be refused")
	}
	if _, err := registry.Certificate(other.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("a revoked device must have no certificate")
	}
	clock.now = clock.now.Add(2 * time.Hour)
	if _, err := registry.Authenticate(device.ID, token); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("an expired device must be refused")
	}
}

func TestCorruptRegistryIsAnErrorNotEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devices.json")
	_ = os.WriteFile(path, []byte("{not json"), 0o600)
	if _, err := OpenRegistry(path, reverseSealer{}, nil); err == nil {
		t.Fatal("a corrupt registry must not load as empty")
	}
}

func TestWindowApproveGivesTokenOnce(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	registry, _ := newTestRegistry(t, clock)
	window := NewWindow(registry, nil, time.Hour, clock.Now)
	if _, err := window.Submit("X", "phone"); !errors.Is(err, ErrNoWindow) {
		t.Fatalf("no window: %v", err)
	}
	code, _ := window.Open(time.Minute)
	pending, err := window.Submit(code, "Pixel")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := window.Submit(code, "Other"); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second phone while one waits must be refused: %v", err)
	}
	if name, ok := window.PendingName(); !ok || name != "Pixel" {
		t.Fatalf("the PC must show the device name: %q", name)
	}
	if err := window.Decide(true); err != nil {
		t.Fatal(err)
	}
	outcome, _ := pending.Wait(context.Background())
	if outcome.Err != nil || outcome.Token == "" {
		t.Fatalf("approved outcome: %+v", outcome)
	}
	if _, err := registry.Authenticate(outcome.Device.ID, outcome.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := window.Submit(code, "Replay"); !errors.Is(err, ErrNoWindow) {
		t.Fatalf("the code is single use: %v", err)
	}
}

func TestWindowDenyWrongCodeAndExpiry(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	registry, _ := newTestRegistry(t, clock)
	window := NewWindow(registry, nil, time.Hour, clock.Now)

	code, _ := window.Open(time.Minute)
	pending, _ := window.Submit(code, "Pixel")
	_ = window.Decide(false)
	if outcome, _ := pending.Wait(context.Background()); !errors.Is(outcome.Err, ErrDenied) || len(registry.List()) != 0 {
		t.Fatalf("deny must add nothing: %+v", outcome)
	}

	code, _ = window.Open(time.Minute)
	for attempt := 1; attempt < MaxCodeAttempts; attempt++ {
		if _, err := window.Submit("WRONG", "x"); !errors.Is(err, ErrBadCode) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if _, err := window.Submit("WRONG", "x"); !errors.Is(err, ErrTooManyTries) {
		t.Fatal("the last wrong code must close the window")
	}
	if _, err := window.Submit(code, "x"); !errors.Is(err, ErrNoWindow) {
		t.Fatal("the right code must not work after the window closed")
	}

	code, _ = window.Open(time.Minute)
	pending, _ = window.Submit(code, "Pixel")
	clock.now = clock.now.Add(2 * time.Minute)
	if err := window.Decide(true); !errors.Is(err, ErrNoWindow) {
		t.Fatalf("approving after expiry: %v", err)
	}
	if outcome, _ := pending.Wait(context.Background()); !errors.Is(outcome.Err, ErrExpired) || len(registry.List()) != 0 {
		t.Fatalf("expiry mid-flow must fail the phone and add nothing: %+v", outcome)
	}
}

func TestReopeningCancelsAWaitingRequest(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	registry, _ := newTestRegistry(t, clock)
	window := NewWindow(registry, nil, time.Hour, clock.Now)
	code, _ := window.Open(time.Minute)
	pending, _ := window.Submit(code, "Pixel")
	if _, err := window.Open(time.Minute); err != nil {
		t.Fatal(err)
	}
	if outcome, _ := pending.Wait(context.Background()); outcome.Err == nil {
		t.Fatal("a replaced window must cancel the earlier request")
	}
}
