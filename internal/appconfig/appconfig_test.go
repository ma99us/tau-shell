package appconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if config, err := Load(path); err != nil || config.Skills.Resolve().Repo == "" || !config.Skills.Resolve().Enabled {
		t.Fatalf("defaults must fetch the cookbook: %+v %v", config.Skills.Resolve(), err)
	}
	good := "version: 1\nskills:\n  repo: \"\"\n  ref: dev\n  auto_update: true\n  local_sources: [" + filepath.Join(t.TempDir(), "cook") + "]\n"
	if err := os.WriteFile(path, []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	resolved := config.Skills.Resolve()
	if err != nil || resolved.Repo != "" || resolved.Ref != "dev" || !resolved.AutoUpdate || len(resolved.LocalSources) != 1 {
		t.Fatalf("resolved = %+v err=%v", resolved, err)
	}
	for _, bad := range []string{
		"skills:\n  repo: http://github.com/a/b\n",
		"skills:\n  repo: https://example.com/a/b\n",
		"skills:\n  ref: ../x\n",
		"skills:\n  local_sources: [relative/path]\n",
		"skills:\n  unknown: 1\n",
	} {
		if err := os.WriteFile(path, []byte("version: 1\n"+bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

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

func TestUsageSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	config, err := Load(path)
	if err != nil || config.Usage.StartDay() != 1 || !config.Usage.PriceUpdatesEnabled() {
		t.Fatalf("defaults: %+v %v", config, err)
	}
	body := "usage:\n  month_start_day: 15\n  price_updates: false\n  claude_prices:\n    opus-5.5: { input: 4, output: 20 }\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err = Load(path)
	if err != nil || config.Usage.StartDay() != 15 || config.Usage.PriceUpdatesEnabled() || config.Usage.ClaudePrices["opus-5.5"].Output != 20 {
		t.Fatalf("usage block: %+v %v", config.Usage, err)
	}
	for _, bad := range []string{"usage:\n  month_start_day: 29\n", "usage:\n  month_start_day: -1\n", "usage:\n  claude_prices:\n    opus-5.5: { input: 4 }\n", "usage:\n  month_starts: 3\n"} {
		if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestQuotaSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	config, err := Load(path)
	if err != nil || !config.Quota.CopilotEnabled() {
		t.Fatalf("default: %+v %v", config, err)
	}
	if err := os.WriteFile(path, []byte("quota:\n  copilot: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err = Load(path)
	if err != nil || config.Quota.CopilotEnabled() {
		t.Fatalf("disabled: %+v %v", config.Quota, err)
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
