package session

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestValidateLaunchArgsStandaloneMatchesConfigValidate(t *testing.T) {
	if err := ValidateLaunchArgs(ClaudeCode, []string{"--dangerously-skip-permissions"}); err != nil {
		t.Fatalf("unreserved argument rejected: %v", err)
	}
	if err := ValidateLaunchArgs(ClaudeCode, []string{"--session-id"}); err == nil || !strings.Contains(err.Error(), "TauShell-managed") {
		t.Fatalf("reserved argument not rejected without a filesystem-backed Config: %v", err)
	}
}

func TestUpdateConfigFieldPreservesCommentsAndValidates(t *testing.T) {
	directory := t.TempDir()
	original := "# my notes\nid: keep-id\nname: Old Name # name inline\n# color description\ncolor: \"#4f8cff\" # color inline\ntool: claude-code\nworking_dir: " + filepath.ToSlash(t.TempDir()) + "\n"
	path := filepath.Join(directory, "keep-id.yaml")
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConfigField(directory, "keep-id", "name", "New: Name #1"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateConfigField(directory, "keep-id", "color", "#4f8cff"); err != nil {
		t.Fatal(err)
	}
	config, err := FindConfig(directory, "keep-id")
	if err != nil {
		t.Fatal(err)
	}
	if config.Name != "New: Name #1" || config.Color != "#4f8cff" || config.ID != "keep-id" {
		t.Fatalf("unexpected config after update: %+v", config)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# my notes") {
		t.Fatalf("comment was lost:\n%s", data)
	}
	if config, err = UpdateConfigField(directory, "keep-id", "color", ""); err != nil || config.Color != "" {
		t.Fatalf("clearing color: %+v, %v", config, err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{"# my notes", "# name inline", "# color description", "# color inline"} {
		if !bytes.Contains(data, []byte(comment)) {
			t.Errorf("comment %q was lost after update/removal:\n%s", comment, data)
		}
	}
	if config, err = UpdateConfigField(directory, "keep-id", "auto_resume", "true"); err != nil || !config.AutoResume {
		t.Fatalf("setting auto_resume: %+v, %v", config, err)
	}
	if config, err = UpdateConfigField(directory, "keep-id", "auto_resume", ""); err != nil || config.AutoResume {
		t.Fatalf("clearing auto_resume: %+v, %v", config, err)
	}
	if _, err := UpdateConfigField(directory, "keep-id", "auto_resume", "yes"); err == nil {
		t.Fatal("invalid auto_resume accepted")
	}
	for _, bad := range [][2]string{{"color", "red"}, {"name", "  "}, {"name", strings.Repeat("x", 81)}, {"tool", "x"}} {
		if _, err := UpdateConfigField(directory, "keep-id", bad[0], bad[1]); err == nil {
			t.Fatalf("UpdateConfigField(%q, %q) should fail", bad[0], bad[1])
		}
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 1 {
		t.Fatalf("unexpected leftover files: %v", entries)
	}
}

func TestReplaceConfigIfUnchangedRejectsExternalEdit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.yaml")
	original := []byte("name: original\n")
	external := []byte("name: external\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, external, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceConfigIfUnchanged(path, original, []byte("name: TauShell\n")); !errors.Is(err, ErrConfigChanged) {
		t.Fatalf("external edit error = %v, want ErrConfigChanged", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, external) {
		t.Fatalf("external file was overwritten: %q, %v", got, err)
	}
}

func TestConcurrentConfigFieldUpdatesPreserveBothChanges(t *testing.T) {
	directory := t.TempDir()
	config := Config{ID: "concurrent", Name: "Before", Tool: ClaudeCode, WorkingDir: t.TempDir()}
	path, err := SaveConfig(directory, config)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsFound := make(chan error, 2)
	var group sync.WaitGroup
	for _, update := range [][2]string{{"name", "After"}, {"auto_resume", "true"}} {
		group.Add(1)
		go func(key, value string) {
			defer group.Done()
			<-start
			_, err := UpdateConfigField(directory, config.ID, key, value)
			errorsFound <- err
		}(update[0], update[1])
	}
	close(start)
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	updated, err := LoadConfig(path)
	if err != nil || updated.Name != "After" || !updated.AutoResume {
		t.Fatalf("concurrent updates = %+v, %v", updated, err)
	}
}

func TestConfigPresetsAreValidated(t *testing.T) {
	workingDir := filepath.ToSlash(t.TempDir())
	base := "id: test-session\nname: Test\ntool: copilot-cli\nworking_dir: " + workingDir + "\n"
	config, err := ParseConfig(base + "presets:\n  - name: Review\n    text: Review {project}\n")
	if err != nil || len(config.Presets) != 1 || config.Presets[0].Name != "Review" {
		t.Fatalf("valid presets: %+v, %v", config.Presets, err)
	}
	for name, body := range map[string]string{
		"duplicate": "presets:\n  - name: A\n    text: x\n  - name: a\n    text: y\n",
		"no text":   "presets:\n  - name: A\n",
	} {
		if _, err := ParseConfig(base + body); err == nil {
			t.Fatalf("%s presets were accepted", name)
		}
	}
}
