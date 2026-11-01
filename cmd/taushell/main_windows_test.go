package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
)

type fixedSessionDiscovery struct {
	harness.Adapter
	id  string
	err error
}

func (adapter fixedSessionDiscovery) DiscoverSessionID(string, time.Time, map[string]bool) (string, error) {
	return adapter.id, adapter.err
}

func (fixedSessionDiscovery) ID() session.Harness { return session.Codex }

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

func TestCLIListRecoversPendingConfigDeletions(t *testing.T) {
	configDirectory := t.TempDir()
	databasePath := filepath.Join(t.TempDir(), "state.db")
	for _, id := range []string{"restore-stage", "finalize-stage"} {
		if _, err := session.SaveConfig(configDirectory, session.Config{
			ID: id, Name: id, Tool: session.ClaudeCode, WorkingDir: t.TempDir(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	store, err := session.OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"restore-stage", "finalize-stage"} {
		path := filepath.Join(configDirectory, id+".yaml")
		if err := os.Rename(path, path+".deleting"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteWith(context.Background(), "finalize-stage", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := listSessionsAt(configDirectory, databasePath, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "restore-stage") || strings.Contains(output.String(), "finalize-stage") {
		t.Fatalf("CLI list after deletion recovery = %q", output.String())
	}
	if _, err := session.FindConfig(configDirectory, "restore-stage"); err != nil {
		t.Fatalf("uncommitted staged definition was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDirectory, "finalize-stage.yaml.deleting")); err == nil {
		t.Fatal("committed tombstone was not finalized")
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
	runtime, err := session.RecoverRuntime(context.Background(), store, "recovery-test")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Status != session.StatusFailed || runtime.ProcessID != 0 || !strings.Contains(runtime.LastError, "exited unexpectedly") {
		t.Fatalf("unexpected recovered runtime state: %+v", runtime)
	}
}

func TestCLISessionDiscoveryReportsAmbiguityAndClaimConflicts(t *testing.T) {
	ctx := context.Background()
	store, err := session.OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	config := session.Config{ID: "cli", Tool: session.Codex, WorkingDir: t.TempDir()}
	cliRuntime, err := store.BeginDiscovered(ctx, config.ID, config.Tool, false, false)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	close(stop)
	note := discoverCLISessionID(store, fixedSessionDiscovery{err: harness.ErrAmbiguousSession}, config, time.Now(), nil, cliRuntime.LaunchToken, stop)
	if !strings.Contains(note, "several codex conversations") {
		t.Fatalf("ambiguous discovery note = %q", note)
	}

	owner, err := store.BeginDiscovered(ctx, "owner", config.Tool, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.ClaimHarnessSessionID(ctx, "owner", config.Tool, owner.LaunchToken, "claimed-id"); err != nil || !claimed {
		t.Fatalf("seed ID claim = %v, %v", claimed, err)
	}
	stop = make(chan struct{})
	close(stop)
	note = discoverCLISessionID(store, fixedSessionDiscovery{id: "claimed-id"}, config, time.Now(), nil, cliRuntime.LaunchToken, stop)
	if !strings.Contains(note, "already linked to another TauShell session") {
		t.Fatalf("claim conflict note = %q", note)
	}
	runtime, err := store.Runtime(ctx, config.ID)
	if err != nil || runtime.HarnessSessionID != "" {
		t.Fatalf("conflicted CLI discovery changed runtime: %+v, %v", runtime, err)
	}
}
