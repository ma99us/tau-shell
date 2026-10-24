//go:build windows

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func (manager *closeTestManager) Delete(context.Context, string) error {
	return errors.New("unexpected Delete call")
}

func (manager *closeTestManager) StopAll(ctx context.Context) error {
	manager.stopAllCalls++
	return manager.stopAll(ctx)
}

func (manager *closeTestManager) Close(context.Context) error {
	return nil
}

func TestTerminalProtocolReports(t *testing.T) {
	input := []byte("sensitive typed text \x1b[0n \x1b[10;1R \x1b[<0;4;3M \x1b[M\x20\x25\x27")
	got := terminalProtocolReports(input)
	want := []string{"device-status", "cursor-position", "mouse-sgr", "mouse-x10"}
	if len(got) != len(want) {
		t.Fatalf("terminalProtocolReports() = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("terminalProtocolReports() = %v, want %v", got, want)
		}
	}
}

func TestTerminalProtocolReportsIgnoreOrdinaryInput(t *testing.T) {
	if got := terminalProtocolReports([]byte("ordinary typed text")); len(got) != 0 {
		t.Fatalf("terminalProtocolReports() = %v, want no protocol reports", got)
	}
	if got := terminalProtocolReports([]byte("\x1b[M")); len(got) != 0 {
		t.Fatalf("terminalProtocolReports() = %v, want incomplete mouse report ignored", got)
	}
}

func TestCreateSessionReturnsSavedSessionWithoutListing(t *testing.T) {
	listErr := errors.New("session listing should not be needed")
	manager := &closeTestManager{
		list: func(context.Context) ([]sessionhost.Snapshot, error) {
			return nil, listErr
		},
	}
	workingDirectory := t.TempDir()
	application := &App{
		manager:         manager,
		configDirectory: filepath.Join(t.TempDir(), "sessions"),
	}
	launchArgs := []string{"--model", "sonnet"}

	snapshot, err := application.CreateSession(" API refactor! ", string(session.ClaudeCode), workingDirectory, launchArgs)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if manager.listCalls != 0 {
		t.Fatalf("session manager List called %d times, want zero", manager.listCalls)
	}
	if snapshot.ID != "api-refactor" || snapshot.Name != "API refactor!" || snapshot.Tool != session.ClaudeCode ||
		snapshot.WorkingDir != workingDirectory || snapshot.Status != session.StatusNeverStarted ||
		snapshot.RunningHere || snapshot.HasResumeID {
		t.Fatalf("created session snapshot = %+v", snapshot)
	}
	config, err := session.FindConfig(application.configDirectory, snapshot.ID)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if len(config.LaunchArgs) != len(launchArgs) || config.LaunchArgs[0] != launchArgs[0] || config.LaunchArgs[1] != launchArgs[1] {
		t.Fatalf("saved launch args = %v, want %v", config.LaunchArgs, launchArgs)
	}
}

func TestCreateSessionDeducesUniqueNameFromProject(t *testing.T) {
	application := &App{configDirectory: filepath.Join(t.TempDir(), "sessions")}
	project := filepath.Join(t.TempDir(), "folder-name")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"name":"My App"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := application.CreateSession("  ", string(session.ClaudeCode), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := application.CreateSession("", string(session.ClaudeCode), project, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "My App" || first.ID != "my-app" || second.Name != "My App (2)" || second.ID != "my-app-2" {
		t.Fatalf("deduced sessions = %+v, %+v", first, second)
	}
	if _, err := application.CreateSession("", string(session.ClaudeCode), "", nil); err == nil {
		t.Fatal("empty name and folder succeeded")
	}
}

func TestSessionYAMLPreviewAndCreateFromEditedYAML(t *testing.T) {
	application := &App{configDirectory: filepath.Join(t.TempDir(), "sessions")}
	project := t.TempDir()

	text, err := application.PreviewSessionYAML("API refactor", string(session.ClaudeCode), project, []string{"--model", "sonnet"})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	for _, want := range []string{"id: api-refactor", "name: API refactor", "tool: claude-code", "- --model", "- sonnet"} {
		if !strings.Contains(text, want) {
			t.Fatalf("preview missing %q:\n%s", want, text)
		}
	}
	if _, err := application.PreviewSessionYAML("", string(session.ClaudeCode), "", nil); err != nil {
		t.Fatalf("blank preview should use a placeholder name: %v", err)
	}

	edited := strings.Replace(text, "name: API refactor", "name: Edited name", 1)
	snapshot, err := application.CreateSessionFromYAML(edited)
	if err != nil {
		t.Fatalf("create from yaml: %v", err)
	}
	if snapshot.ID != "api-refactor" || snapshot.Name != "Edited name" || snapshot.Status != session.StatusNeverStarted {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if _, err := application.CreateSessionFromYAML(edited); err == nil {
		t.Fatal("duplicate id from YAML succeeded")
	}
	for name, bad := range map[string]string{
		"unknown field": strings.Replace(text, "tool:", "bogus: 1\ntool:", 1),
		"reserved arg":  strings.Replace(text, "- sonnet", "- sonnet\n    - --resume", 1),
		"bad tool":      strings.Replace(text, "claude-code", "nope", 1),
		"two documents": text + "---\n" + text,
	} {
		if _, err := application.CreateSessionFromYAML(bad); err == nil {
			t.Fatalf("%s: create from invalid YAML succeeded", name)
		}
	}
}

func TestCreateSessionRejectsInvalidConfig(t *testing.T) {
	application := &App{
		configDirectory: filepath.Join(t.TempDir(), "sessions"),
	}

	_, err := application.CreateSession("API refactor", string(session.ClaudeCode), t.TempDir(), []string{"--session-id=override"})
	if err == nil || !strings.Contains(err.Error(), "launch_args cannot override TauShell-managed --session-id") {
		t.Fatalf("CreateSession error = %v, want reserved launch argument validation error", err)
	}
	if _, err := os.Stat(filepath.Join(application.configDirectory, "api-refactor.yaml")); !os.IsNotExist(err) {
		t.Fatalf("invalid session config file stat error = %v, want file not to exist", err)
	}
}

func TestGetSessionTemplateReturnsAdapterDefaults(t *testing.T) {
	application := &App{}
	for _, test := range []struct {
		tool string
		want []string
	}{
		{string(session.ClaudeCode), []string{"--dangerously-skip-permissions"}},
		{string(session.CopilotCLI), []string{"--yolo"}},
	} {
		template, err := application.GetSessionTemplate(test.tool)
		if err != nil {
			t.Fatalf("GetSessionTemplate(%q): %v", test.tool, err)
		}
		if template.Tool != session.Harness(test.tool) || strings.Join(template.LaunchArgs, ",") != strings.Join(test.want, ",") {
			t.Fatalf("GetSessionTemplate(%q) = %+v, want launchArgs %v", test.tool, template, test.want)
		}
	}
}

func TestGetSessionTemplateRejectsUnsupportedTool(t *testing.T) {
	application := &App{}
	if _, err := application.GetSessionTemplate("unknown-tool"); err == nil {
		t.Fatal("GetSessionTemplate(\"unknown-tool\") should fail")
	}
}

func TestPreviewSessionLaunchRejectsReservedArgumentWithoutTouchingPATH(t *testing.T) {
	application := &App{}
	_, err := application.PreviewSessionLaunch(string(session.ClaudeCode), []string{"--session-id=override"})
	if err == nil || !strings.Contains(err.Error(), "launch_args cannot override TauShell-managed --session-id") {
		t.Fatalf("PreviewSessionLaunch error = %v, want reserved launch argument validation error", err)
	}
}

func TestPreviewSessionLaunchAppendsManagedIdentityPlaceholder(t *testing.T) {
	application := &App{}
	preview, err := application.PreviewSessionLaunch(string(session.CopilotCLI), []string{" --yolo ", "", "--model", "gpt-5"})
	if err != nil {
		t.Fatalf("PreviewSessionLaunch: %v", err)
	}
	want := []string{"--yolo", "--model", "gpt-5", "--session-id", previewSessionIDPlaceholder}
	if strings.Join(preview.Arguments, ",") != strings.Join(want, ",") {
		t.Fatalf("PreviewSessionLaunch arguments = %v, want %v", preview.Arguments, want)
	}
	if preview.Executable == "" {
		t.Fatal("PreviewSessionLaunch executable must not be empty")
	}
	if preview.Warnings == nil {
		t.Fatal("PreviewSessionLaunch warnings must be a non-nil slice for stable JSON serialization")
	}
}

func TestPreviewSessionLaunchRejectsUnsupportedTool(t *testing.T) {
	application := &App{}
	if _, err := application.PreviewSessionLaunch("unknown-tool", nil); err == nil {
		t.Fatal("PreviewSessionLaunch(\"unknown-tool\", ...) should fail")
	}
}

func TestCreateSessionRejectsDuplicateIDWithoutOverwriting(t *testing.T) {
	configDirectory := filepath.Join(t.TempDir(), "sessions")
	workingDirectory := t.TempDir()
	original := session.Config{
		ID:         "api-refactor",
		Name:       "Original session",
		Tool:       session.ClaudeCode,
		WorkingDir: workingDirectory,
	}
	configPath, err := session.SaveConfig(configDirectory, original)
	if err != nil {
		t.Fatalf("save original config: %v", err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read original config: %v", err)
	}

	application := &App{configDirectory: configDirectory}
	_, err = application.CreateSession("API refactor", string(session.CopilotCLI), workingDirectory, []string{"--agent", "new"})
	if err == nil || !strings.Contains(err.Error(), "create session config") {
		t.Fatalf("CreateSession error = %v, want duplicate session-file error", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after duplicate create: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("duplicate session creation modified config:\nbefore:\n%s\nafter:\n%s", before, after)
	}
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
	if !application.confirmShutdown(context.Background()) {
		t.Fatal("beforeClose allowed closing after session state could not be checked")
	}
	if !dialogShown {
		t.Fatal("session check failure was not shown")
	}
}

func TestBeforeCloseDefersConfirmationAndAllowsConfirmedRetry(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()

	manager := &closeTestManager{
		list: func(context.Context) ([]sessionhost.Snapshot, error) {
			started <- struct{}{}
			<-release
			return nil, nil
		},
		stopAll: func(context.Context) error {
			return nil
		},
	}
	requestedQuit := make(chan struct{}, 1)
	application := &App{
		manager: manager,
		requestQuit: func(context.Context) {
			requestedQuit <- struct{}{}
		},
	}
	ctx := context.Background()

	if !application.beforeClose(ctx) {
		t.Fatal("initial close request was not deferred")
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown confirmation did not start")
	}
	if !application.beforeClose(ctx) {
		t.Fatal("repeated close request was not deferred during confirmation")
	}

	close(release)
	released = true
	select {
	case <-requestedQuit:
	case <-time.After(3 * time.Second):
		t.Fatal("confirmed shutdown did not request application close")
	}
	if application.beforeClose(ctx) {
		t.Fatal("confirmed retry did not allow the application to close")
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
			if options.DefaultButton != "No" || options.CancelButton != "No" {
				t.Errorf("shutdown dialog defaults = %q/%q, want No/No", options.DefaultButton, options.CancelButton)
			}
			if len(options.Buttons) != 2 || options.Buttons[0] != "Yes" || options.Buttons[1] != "No" {
				t.Errorf("shutdown dialog buttons = %v, want Yes/No", options.Buttons)
			}
			return "No", nil
		},
	}

	if !application.confirmShutdown(context.Background()) {
		t.Fatal("cancelling shutdown allowed TauShell to close")
	}
	if manager.stopAllCalls != 0 {
		t.Fatalf("StopAll called %d times after cancellation, want zero", manager.stopAllCalls)
	}
}

func TestBeforeCloseKeepsWindowOpenWhenConfirmationDialogFails(t *testing.T) {
	dialogErr := errors.New("native confirmation dialog unavailable")
	manager := &closeTestManager{
		list: func(context.Context) ([]sessionhost.Snapshot, error) {
			return []sessionhost.Snapshot{{Name: "API session", RunningHere: true, Status: session.StatusRunning}}, nil
		},
		stopAll: func(context.Context) error {
			t.Fatal("StopAll called after shutdown confirmation failed")
			return nil
		},
	}
	application := &App{
		manager: manager,
		showMessageDialog: func(context.Context, runtime.MessageDialogOptions) (string, error) {
			return "", dialogErr
		},
	}

	if !application.confirmShutdown(context.Background()) {
		t.Fatal("confirmation-dialog failure allowed TauShell to close")
	}
	if manager.stopAllCalls != 0 {
		t.Fatalf("StopAll called %d times after confirmation failure, want zero", manager.stopAllCalls)
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
				if options.DefaultButton != "No" || options.CancelButton != "No" {
					t.Errorf("shutdown dialog defaults = %q/%q, want No/No", options.DefaultButton, options.CancelButton)
				}
				if len(options.Buttons) != 2 || options.Buttons[0] != "Yes" || options.Buttons[1] != "No" {
					t.Errorf("shutdown dialog buttons = %v, want Yes/No", options.Buttons)
				}
				return "Yes", nil
			}
			if options.Title == "Some sessions required force termination" && !strings.Contains(options.Message, "API session") {
				t.Errorf("forced-shutdown notice omitted session name: %q", options.Message)
			}
			return "", nil
		},
	}

	if application.confirmShutdown(context.Background()) {
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
				return "Yes", nil
			}
			if options.Title == "Some sessions are still running" && strings.Contains(options.Message, "API session") {
				failureShown = true
			}
			return "", nil
		},
	}

	if !application.confirmShutdown(context.Background()) {
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
				return "Yes", nil
			}
			if options.Title == "Unable to stop all sessions" && strings.Contains(options.Message, stopErr.Error()) {
				failureShown = true
			}
			return "", nil
		},
	}

	if !application.confirmShutdown(context.Background()) {
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
