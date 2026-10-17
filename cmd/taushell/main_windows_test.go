package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/session"
)

func TestHelpIsAvailableWithoutUserConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "taushell restore <session-id>") {
		t.Fatalf("help is missing restore command: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("help wrote unexpected error output: %q", stderr.String())
	}
}

func TestSessionCommandArgsPreserveSessionIDAndAction(t *testing.T) {
	for _, test := range []struct {
		command     string
		wantID      string
		wantRestore bool
	}{
		{command: "start", wantID: "backend-agent"},
		{command: "restore", wantID: "backend-agent", wantRestore: true},
	} {
		id, restoring := sessionCommandArgs(test.command, test.wantID)
		if id != test.wantID || restoring != test.wantRestore {
			t.Errorf("%s arguments = (%q, %t), want (%q, %t)", test.command, id, restoring, test.wantID, test.wantRestore)
		}
	}
}

func TestListShowsConfiguredSessionsAndRuntimeStatus(t *testing.T) {
	configDirectory := t.TempDir()
	config := "id: reviewer\nname: Code Reviewer\ntool: copilot-cli\nworking_dir: " + filepath.ToSlash(t.TempDir()) + "\n"
	if err := os.WriteFile(filepath.Join(configDirectory, "reviewer.yaml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := session.OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(t.Context(), "reviewer", session.CopilotCLI, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := listSessionsAt(configDirectory, databasePath, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "reviewer") || !strings.Contains(output.String(), "copilot-cli") || !strings.Contains(output.String(), "starting") {
		t.Fatalf("session listing lacks expected fields: %q", output.String())
	}
}

func TestRecoveryMarksExitedProcessAsFailed(t *testing.T) {
	store, err := session.OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.Begin(context.Background(), "recovery-test", session.ClaudeCode, false); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkRunning(context.Background(), "recovery-test", int(^uint32(0)>>1)); err != nil {
		t.Fatal(err)
	}
	runtime, err := recoverRuntime(context.Background(), store, "recovery-test")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Status != session.StatusFailed || runtime.ProcessID != 0 || !strings.Contains(runtime.LastError, "exited unexpectedly") {
		t.Fatalf("unexpected recovered runtime state: %+v", runtime)
	}
}
