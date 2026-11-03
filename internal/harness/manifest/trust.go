package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Trust remembers which user manifests the user confirmed. A confirmation is
// bound to the manifest's content hash, so editing the file asks again.
// Built-in manifests are always trusted.
type Trust struct {
	mu       sync.Mutex
	path     string
	approved map[string]string
}

// NewTrust returns an empty store that saves to path; nothing is confirmed.
func NewTrust(path string) *Trust {
	return &Trust{path: path, approved: map[string]string{}}
}

// OpenTrust reads the trust file. A missing file means nothing is confirmed.
func OpenTrust(path string) (*Trust, error) {
	trust := NewTrust(path)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return trust, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read harness trust file: %w", err)
	}
	if err := json.Unmarshal(data, &trust.approved); err != nil {
		return nil, fmt.Errorf("parse harness trust file %s: %w", path, err)
	}
	return trust, nil
}

// Trusted reports whether the manifest may be launched.
func (trust *Trust) Trusted(item *Manifest) bool {
	if item == nil {
		return false
	}
	if item.Builtin {
		return true
	}
	if trust == nil {
		return false
	}
	trust.mu.Lock()
	defer trust.mu.Unlock()
	return item.Hash != "" && trust.approved[item.ID] == item.Hash
}

// Approve records the user's confirmation of this exact manifest content.
func (trust *Trust) Approve(item *Manifest) error {
	if item == nil || item.Builtin {
		return errors.New("only user harness manifests need confirmation")
	}
	trust.mu.Lock()
	defer trust.mu.Unlock()
	previous, had := trust.approved[item.ID]
	trust.approved[item.ID] = item.Hash
	data, err := json.MarshalIndent(trust.approved, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(trust.path), 0o700)
	}
	if err == nil {
		err = os.WriteFile(trust.path, append(data, '\n'), 0o600)
	}
	if err != nil {
		if had {
			trust.approved[item.ID] = previous
		} else {
			delete(trust.approved, item.ID)
		}
		return fmt.Errorf("save harness trust file: %w", err)
	}
	return nil
}

var (
	defaultTrustMu sync.RWMutex
	defaultTrust   *Trust
)

// SetDefaultTrust installs the trust store used by Allowed.
func SetDefaultTrust(trust *Trust) {
	defaultTrustMu.Lock()
	defaultTrust = trust
	defaultTrustMu.Unlock()
}

// DefaultTrust returns the installed trust store, which may be nil.
func DefaultTrust() *Trust {
	defaultTrustMu.RLock()
	defer defaultTrustMu.RUnlock()
	return defaultTrust
}

// Allowed reports whether the harness may be launched, with the reason when
// it may not.
func Allowed(id string) error {
	item, ok := Get(id)
	if !ok {
		return fmt.Errorf("unsupported harness %q", id)
	}
	if !DefaultTrust().Trusted(item) {
		return fmt.Errorf("harness %q is defined by %s and has not been confirmed; open TauShell Home and confirm it first", id, item.Path)
	}
	return nil
}
