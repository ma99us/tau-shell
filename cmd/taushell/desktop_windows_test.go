//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ma99us/tau-shell/internal/sessionhost"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
