package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	config, err := Load(path)
	if err != nil || !config.Notifications.ToastEnabled() || !config.Notifications.TaskbarFlashEnabled() {
		t.Fatalf("missing file should mean defaults on: %+v %v", config, err)
	}
	if err := os.WriteFile(path, []byte("version: 1\nnotifications:\n  toast: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err = Load(path)
	if err != nil || config.Notifications.ToastEnabled() || !config.Notifications.TaskbarFlashEnabled() {
		t.Fatalf("flags: %+v %v", config, err)
	}
}

func TestLoadRejectsTyposAndVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	for _, body := range []string{"notifications:\n  toasts: true\n", "version: 9\n"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestEnsureExampleNeverOverwritesAndParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := EnsureExample(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("example does not parse: %v", err)
	}
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureExample(path); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "version: 1\n" {
		t.Fatal("existing file was overwritten")
	}
}
