// Package appconfig loads the hand-edited, app-wide TauShell settings file.
package appconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const FileName = "config.yaml"

// Notifications controls local attention alerts. A nil flag means enabled.
type Notifications struct {
	Toast        *bool `yaml:"toast,omitempty"`
	TaskbarFlash *bool `yaml:"taskbar_flash,omitempty"`
}

type Config struct {
	Version       int           `yaml:"version"`
	Notifications Notifications `yaml:"notifications"`
}

func (n Notifications) ToastEnabled() bool        { return n.Toast == nil || *n.Toast }
func (n Notifications) TaskbarFlashEnabled() bool { return n.TaskbarFlash == nil || *n.TaskbarFlash }

// Load reads the settings file. A missing file means defaults; invalid
// content is an error so typos are never silently ignored.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read settings: %w", err)
	}
	var parsed Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if parsed.Version != 0 && parsed.Version != 1 {
		return Config{}, fmt.Errorf("%s: unsupported version %d", filepath.Base(path), parsed.Version)
	}
	return parsed, nil
}

const exampleFile = `# TauShell settings. Edit and save; changes apply to the next alert.
version: 1

notifications:
  # Windows toast when a session needs your answer or finishes while TauShell is in the background.
  toast: true
  # Flash the TauShell taskbar button for the same events.
  taskbar_flash: true
`

// EnsureExample creates the commented example file only when none exists.
func EnsureExample(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(exampleFile)
	return errors.Join(writeErr, file.Close())
}
