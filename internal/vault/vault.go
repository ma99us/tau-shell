// Package vault keeps named secrets outside session YAML and runtime state.
// Values leave the vault only to be placed in a session's process environment.
package vault

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

// MaxValueBytes is the Windows Credential Manager blob limit.
const MaxValueBytes = 2560

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ErrNotFound reports that no secret has the requested name.
var ErrNotFound = errors.New("secret not found")

// Store is a named secret store. Implementations must never log values.
type Store interface {
	Get(name string) (string, error)
	Set(name, value string) error
	Delete(name string) error
	// Names lists stored secret names, sorted. It never returns values.
	Names() ([]string, error)
}

// ValidateName checks a secret name.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("secret name %q must be 1-64 letters, digits, '.', '_' or '-', starting with a letter or digit", name)
	}
	return nil
}

func validateValue(value string) error {
	if value == "" {
		return errors.New("secret value must not be empty")
	}
	if len(value) > MaxValueBytes {
		return fmt.Errorf("secret value is longer than %d bytes", MaxValueBytes)
	}
	return nil
}

// Memory is a Store held in process memory, for tests and non-Windows builds.
type Memory struct {
	mu     sync.Mutex
	values map[string]string
}

func NewMemory() *Memory { return &Memory{values: map[string]string{}} }

func (memory *Memory) Get(name string) (string, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	value, ok := memory.values[name]
	if !ok {
		return "", ErrNotFound
	}
	return value, nil
}

func (memory *Memory) Set(name, value string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := validateValue(value); err != nil {
		return err
	}
	memory.mu.Lock()
	defer memory.mu.Unlock()
	memory.values[name] = value
	return nil
}

func (memory *Memory) Delete(name string) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	if _, ok := memory.values[name]; !ok {
		return ErrNotFound
	}
	delete(memory.values, name)
	return nil
}

func (memory *Memory) Names() ([]string, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	names := make([]string, 0, len(memory.values))
	for name := range memory.values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
