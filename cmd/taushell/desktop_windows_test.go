//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/sessionhost"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type closeTestManager struct {
	list         func(context.Context) ([]sessionhost.Snapshot, error)
	stopAll      func(context.Context) error
	listCalls    int
	stopAllCalls int
}

func (manager *closeTestManager) List(ctx context.Context) ([]sessionhost.Snapshot, error) {
	manager.listCalls++
	return manager.list(ctx)
}

func (manager *closeTestManager) Start(context.Context, string, bool, int, int) (sessionhost.Snapshot, error) {
	return sessionhost.Snapshot{}, errors.New("unexpected Start call")
}

func (manager *closeTestManager) StartFresh(context.Context, string, int, int) (sessionhost.Snapshot, error) {
	return sessionhost.Snapshot{}, errors.New("unexpected StartFresh call")
}

func (manager *closeTestManager) Write(string, []byte) error {
	return errors.New("unexpected Write call")
}

func (manager *closeTestManager) Resize(string, int, int) error {
	return errors.New("unexpected Resize call")
}

func (manager *closeTestManager) Stop(context.Context, string) error {
	return errors.New("unexpected Stop call")
}

func (manager *closeTestManager) StopAll(ctx context.Context) error {
	manager.stopAllCalls++
	return manager.stopAll(ctx)
}

func (manager *closeTestManager) Close(context.Context) error {
	return nil
}

func TestBeforeCloseKeepsWindowOpenWhenSessionCheckFails(t *testing.T) {
	root := t.TempDir()
	configDirectory := filepath.Join(root, "sessions")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDirectory, "invalid.yaml"), []byte("id: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := sessionhost.New(configDirectory, filepath.Join(root, "state.db"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(context.Background())

	dialogShown := false
	application := &App{
		manager: manager,
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			dialogShown = true
			if options.Title != "Unable to check active sessions" {
				t.Errorf("dialog title = %q, want active-session check failure", options.Title)
			}
			return "", nil
		},
	}
	if !application.beforeClose(context.Background()) {
		t.Fatal("beforeClose allowed closing after session state could not be checked")
	}
	if !dialogShown {
		t.Fatal("session check failure was not shown")
	}
}

func TestBeforeCloseCancelKeepsRunningSessions(t *testing.T) {
	manager := &closeTestManager{
		list: func(context.Context) ([]sessionhost.Snapshot, error) {
			return []sessionhost.Snapshot{{Name: "API session", RunningHere: true, Status: session.StatusRunning}}, nil
		},
		stopAll: func(context.Context) error {
			t.Fatal("StopAll called after the user chose to keep TauShell open")
			return nil
		},
	}
	application := &App{
		manager: manager,
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			if options.Title != "Sessions are still running" {
				t.Errorf("dialog title = %q, want running-session confirmation", options.Title)
			}
			return "Keep TauShell open", nil
		},
	}

	if !application.beforeClose(context.Background()) {
		t.Fatal("cancelling shutdown allowed TauShell to close")
	}
	if manager.stopAllCalls != 0 {
		t.Fatalf("StopAll called %d times after cancellation, want zero", manager.stopAllCalls)
	}
}

func TestBeforeCloseStopsSessionsAndReportsForcedTermination(t *testing.T) {
	manager := &closeTestManager{stopAll: func(context.Context) error { return nil }}
	manager.list = func(context.Context) ([]sessionhost.Snapshot, error) {
		if manager.listCalls == 1 {
			return []sessionhost.Snapshot{{Name: "API session", RunningHere: true, Status: session.StatusRunning}}, nil
		}
		return []sessionhost.Snapshot{{Name: "API session", Status: session.StatusStopped, LastError: "force-terminated"}}, nil
	}
	var dialogTitles []string
	application := &App{
		manager: manager,
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			dialogTitles = append(dialogTitles, options.Title)
			if options.Title == "Sessions are still running" {
				return "Stop sessions and close", nil
			}
			if options.Title == "Some sessions required force termination" && !strings.Contains(options.Message, "API session") {
				t.Errorf("forced-shutdown notice omitted session name: %q", options.Message)
			}
			return "", nil
		},
	}

	if application.beforeClose(context.Background()) {
		t.Fatal("successful confirmed shutdown prevented closing")
	}
	if manager.stopAllCalls != 1 || manager.listCalls != 2 {
		t.Fatalf("shutdown calls: StopAll=%d List=%d, want 1 and 2", manager.stopAllCalls, manager.listCalls)
	}
	if len(dialogTitles) != 2 || dialogTitles[0] != "Sessions are still running" || dialogTitles[1] != "Some sessions required force termination" {
		t.Fatalf("dialog sequence = %v", dialogTitles)
	}
}

func TestBeforeCloseKeepsWindowOpenWhenSessionRemainsRunning(t *testing.T) {
	manager := &closeTestManager{
		list: func(context.Context) ([]sessionhost.Snapshot, error) {
			return []sessionhost.Snapshot{{Name: "API session", RunningHere: true, Status: session.StatusRunning}}, nil
		},
		stopAll: func(context.Context) error { return nil },
	}
	var failureShown bool
	application := &App{
		manager: manager,
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			if options.Title == "Sessions are still running" {
				return "Stop sessions and close", nil
			}
			if options.Title == "Some sessions are still running" && strings.Contains(options.Message, "API session") {
				failureShown = true
			}
			return "", nil
		},
	}

	if !application.beforeClose(context.Background()) {
		t.Fatal("post-stop verification allowed closing while a managed session remained running")
	}
	if !failureShown {
		t.Fatal("remaining running session was not reported")
	}
	if manager.stopAllCalls != 1 || manager.listCalls != 2 {
		t.Fatalf("shutdown calls: StopAll=%d List=%d, want 1 and 2", manager.stopAllCalls, manager.listCalls)
	}
}

func TestBeforeCloseKeepsWindowOpenWhenStoppingSessionsFails(t *testing.T) {
	stopErr := errors.New("one session did not stop")
	manager := &closeTestManager{
		list: func(context.Context) ([]sessionhost.Snapshot, error) {
			return []sessionhost.Snapshot{{Name: "API session", RunningHere: true, Status: session.StatusRunning}}, nil
		},
		stopAll: func(context.Context) error { return stopErr },
	}
	var failureShown bool
	application := &App{
		manager: manager,
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			if options.Title == "Sessions are still running" {
				return "Stop sessions and close", nil
			}
			if options.Title == "Unable to stop all sessions" && strings.Contains(options.Message, stopErr.Error()) {
				failureShown = true
			}
			return "", nil
		},
	}

	if !application.beforeClose(context.Background()) {
		t.Fatal("stop failure allowed TauShell to close")
	}
	if !failureShown {
		t.Fatal("stop failure was not reported")
	}
	if manager.stopAllCalls != 1 || manager.listCalls != 1 {
		t.Fatalf("shutdown calls: StopAll=%d List=%d, want 1 each", manager.stopAllCalls, manager.listCalls)
	}
}

func TestForcedShutdownWarningFailureIsRetriedBeforeClose(t *testing.T) {
	var messages []string
	dialogErr := errors.New("native dialog unavailable")
	application := &App{
		showMessageDialog: func(_ context.Context, options runtime.MessageDialogOptions) (string, error) {
			messages = append(messages, options.Message)
			if len(messages) == 1 {
				return "", dialogErr
			}
			return "", nil
		},
	}

	if !application.reportForcedShutdowns(context.Background(), []string{"API session"}) {
		t.Fatal("failed warning dialog allowed the app to close")
	}
	if len(application.forcedShutdownWarnings) != 1 || application.forcedShutdownWarnings[0] != "API session" {
		t.Fatalf("pending warnings after dialog failure = %v", application.forcedShutdownWarnings)
	}
	if application.reportForcedShutdowns(context.Background(), nil) {
		t.Fatal("successful warning retry prevented closing")
	}
	if application.forcedShutdownWarnings != nil {
		t.Fatalf("pending warnings were not cleared after successful report: %v", application.forcedShutdownWarnings)
	}
	if len(messages) != 2 || !strings.Contains(messages[1], "API session") {
		t.Fatalf("warning retry did not include the forced session: %v", messages)
	}
}
