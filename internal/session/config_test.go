package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigAndFilenameIdentity(t *testing.T) {
	directory := t.TempDir()
	workingDir := t.TempDir()
	yaml := "id: backend-agent\nname: Backend Agent\ntool: claude-code\nworking_dir: " + filepath.ToSlash(workingDir) + "\nenvironment:\n  NODE_NO_WARNINGS: \"1\"\npowershell:\n  utf8: true\n"
	path := filepath.Join(directory, "backend-agent.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := FindConfig(directory, "backend-agent")
	if err != nil {
		t.Fatal(err)
	}
	if config.ID != "backend-agent" || config.Tool != ClaudeCode || config.Environment["NODE_NO_WARNINGS"] != "1" {
		t.Fatalf("unexpected config: %+v", config)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != yaml {
		t.Fatal("loading configuration modified the human-edited YAML file")
	}
}

func TestLoadConfigRejectsUnknownFieldsAndInvalidEnvironmentName(t *testing.T) {
	workingDir := filepath.ToSlash(t.TempDir())
	base := "id: test-session\nname: Test\ntool: copilot-cli\nworking_dir: " + workingDir + "\n%s"
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{"unknown field", "resume_latest: true\n", "field resume_latest not found"},
		{"invalid environment variable", "environment:\n  BAD=NAME: value\n", "invalid environment variable name"},
		{"invalid ID", "environment:\n  VALID: value\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Replace(base, "%s", test.body, 1)
			if test.name == "invalid ID" {
				body = strings.Replace(body, "test-session", "../bad", 1)
				test.want = "lowercase slug"
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConfig(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestListConfigsRequiresFilenameToMatchID(t *testing.T) {
	directory := t.TempDir()
	content := "id: another-id\nname: Test\ntool: claude-code\nworking_dir: " + filepath.ToSlash(t.TempDir()) + "\n"
	if err := os.WriteFile(filepath.Join(directory, "wrong-name.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ListConfigs(directory); err == nil || !strings.Contains(err.Error(), "must be") {
		t.Fatalf("expected mismatched filename error, got %v", err)
	}
}

func TestSaveConfigCreatesHumanEditableFileWithoutOverwriting(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "sessions")
	config := Config{
		ID:         "custom-session",
		Name:       "Custom Session",
		Tool:       CopilotCLI,
		WorkingDir: t.TempDir(),
		LaunchArgs: []string{"--yolo", "--model", "gpt-5"},
	}
	path, err := SaveConfig(directory, config)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := FindConfig(directory, config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.LaunchArgs) != 3 || loaded.LaunchArgs[0] != "--yolo" || loaded.LaunchArgs[2] != "gpt-5" {
		t.Fatalf("saved launch arguments = %#v", loaded.LaunchArgs)
	}
	if _, err := SaveConfig(directory, config); err == nil {
		t.Fatal("saving an existing session config should not overwrite it")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved config not found: %v", err)
	}
}

func TestConfigRejectsArgumentsThatOverrideManagedSessionIdentity(t *testing.T) {
	for _, test := range []struct {
		tool Harness
		arg  string
	}{
		{ClaudeCode, "--session-id"},
		{ClaudeCode, "--resume=other"},
		{CopilotCLI, "--session-id=other"},
		{CopilotCLI, "--resume"},
	} {
		config := Config{
			ID:         "reserved-argument",
			Name:       "Reserved argument",
			Tool:       test.tool,
			WorkingDir: t.TempDir(),
			LaunchArgs: []string{test.arg},
		}
		if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "TauShell-managed") {
			t.Errorf("%s %q should be rejected, got %v", test.tool, test.arg, err)
		}
	}
}
