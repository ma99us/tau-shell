//go:build windows

package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/sessionhost"
	"github.com/ma99us/tau-shell/internal/skills"
	"github.com/ma99us/tau-shell/internal/tools"
	"github.com/ma99us/tau-shell/internal/usage"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsOptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed frontend/dist
var frontendAssets embed.FS

var sessionIDPattern = regexp.MustCompile(`[^a-z0-9]+`)

type App struct {
	manager                desktopSessionManager
	ctx                    context.Context
	configDirectory        string
	webViewVersion         string
	showMessageDialog      func(context.Context, runtime.MessageDialogOptions) (string, error)
	emitEvent              func(name string, data ...any)
	dialogs                dialogBroker
	requestQuit            func(context.Context)
	forcedShutdownWarnings []string
	terminalReports        sync.Map
	shutdownMu             sync.Mutex
	shutdownInProgress     bool
	allowShutdown          bool
	installMu              sync.Mutex
	updatesMu              sync.Mutex
	toolUpdates            map[string]string
	skills                 *skills.Library
	toolInstaller          *tools.Installer
	openURL                func(context.Context, string)
	openFile               func(string) error
	attention              attentionState
	attentionHooks         *attentionHooks
}

type desktopSessionManager interface {
	List(context.Context) ([]sessionhost.Snapshot, error)
	HarnessModel(context.Context, string) (string, error)
	SessionUsage(context.Context, string) (usage.SessionUsage, error)
	UsageSummary(context.Context) (usage.Summary, error)
	StartPriceUpdates(context.Context)
	Start(context.Context, string, bool, int, int) (sessionhost.Snapshot, error)
	StartFresh(context.Context, string, int, int) (sessionhost.Snapshot, error)
	Write(string, []byte) error
	Resize(string, int, int) error
	Stop(context.Context, string) error
	Delete(context.Context, string) error
	UpdateDisplay(id, field, value string) error
	AutoResumeAll(ctx context.Context) (bool, error)
	SetAutoResumeAll(ctx context.Context, enabled bool) error
	SetOrder(context.Context, []string) error
	PlaceAfter(ctx context.Context, id, afterID string) error
	StopAll(context.Context) error
	Close(context.Context) error
}

type SystemStatus struct {
	WebView2Version    string         `json:"webView2Version"`
	DiagnosticsLogPath string         `json:"diagnosticsLogPath"`
	WingetAvailable    bool           `json:"wingetAvailable"`
	NpmAvailable       bool           `json:"npmAvailable"`
	Tools              []tools.Status `json:"tools"`
}

func runDesktop() error {
	diagnostics.Info("desktop startup beginning", "version", version)
	version, err := installedWebView2Version()
	if err != nil {
		diagnostics.Error("check WebView2 runtime failed", err)
		return fmt.Errorf("check Microsoft Edge WebView2 Runtime: %w", err)
	}
	if version == "" {
		diagnostics.Warn("WebView2 runtime is missing", errors.New("required desktop prerequisite not found"))
		return explainMissingWebView2()
	}

	configDirectory, databasePath, err := appPaths()
	if err != nil {
		diagnostics.Error("resolve TauShell data paths failed", err)
		return err
	}
	assetFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		diagnostics.Error("load embedded desktop assets failed", err)
		return fmt.Errorf("load embedded desktop assets: %w", err)
	}
	application := &App{configDirectory: configDirectory, webViewVersion: version}
	manager, err := sessionhost.New(configDirectory, databasePath, application.publish)
	if err != nil {
		diagnostics.Error("initialize session manager failed", err)
		return fmt.Errorf("initialize session manager: %w", err)
	}
	application.manager = manager
	application.initSkills(manager)
	diagnostics.Info("desktop window starting", "webview2_version", version)
	runErr := wails.Run(&options.App{
		Title:              "TauShell",
		Width:              1280,
		Height:             920,
		MinWidth:           760,
		MinHeight:          520,
		WindowStartState:   options.Normal,
		StartHidden:        true,
		AssetServer:        &assetserver.Options{Assets: assetFS},
		Bind:               []interface{}{application},
		Logger:             diagnostics.WailsLogger{},
		LogLevel:           logger.DEBUG,
		LogLevelProduction: logger.DEBUG,
		OnNavigationStarting: func(_ context.Context, targetURL string) bool {
			if isBundledAppNavigation(targetURL) {
				return true
			}
			diagnostics.Warn("blocked WebView navigation outside bundled app origin", errors.New("navigation denied"))
			return false
		},
		OnStartup:     application.startup,
		OnDomReady:    application.restoreWindowPlacement,
		OnBeforeClose: application.beforeClose,
		OnShutdown:    application.shutdown,
		Windows: &windowsOptions.Options{
			Theme: windowsOptions.Dark,
		},
	})
	closeCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	closeErr := application.manager.Close(closeCtx)
	if runErr != nil {
		diagnostics.Error("Wails desktop runtime stopped with an error", runErr)
	}
	if closeErr != nil {
		diagnostics.Error("close session manager failed", closeErr)
	}
	diagnostics.Info("desktop shutdown complete", "runtime_error", runErr != nil, "manager_close_error", closeErr != nil)
	return errors.Join(runErr, closeErr)
}

func (application *App) startup(ctx context.Context) {
	application.ctx = ctx
	application.initAttention()
	diagnostics.Info("desktop UI ready", "webview2_version", application.webViewVersion)
	application.manager.StartPriceUpdates(ctx)
	application.startToolUpdateChecks(ctx)
	application.startSkillChecks(ctx)
}

func (application *App) publish(event sessionhost.Event) {
	if event.Kind == sessionhost.EventState && event.Session.ID != "" {
		diagnostics.Info("session state changed",
			"session_id", event.SessionID,
			"status", event.Session.Status,
			"running_here", event.Session.RunningHere,
			"has_resume_id", event.Session.HasResumeID,
			"has_error", event.Session.LastError != "",
		)
	}
	if application.ctx != nil {
		runtime.EventsEmit(application.ctx, "session-event", event)
	}
}

func (application *App) GetSessions() (snapshots []sessionhost.Snapshot, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("list sessions failed", err)
			return
		}
		diagnostics.Debug("sessions listed", "count", len(snapshots))
	}()
	return application.manager.List(context.Background())
}

func (application *App) GetSystemStatus() SystemStatus {
	_, wingetErr := exec.LookPath("winget.exe")
	_, npmErr := exec.LookPath("npm.cmd")
	status := SystemStatus{
		WebView2Version:    application.webViewVersion,
		DiagnosticsLogPath: diagnostics.LogPath(),
		WingetAvailable:    wingetErr == nil,
		NpmAvailable:       npmErr == nil,
		Tools:              tools.DetectAll(context.Background(), tools.DefaultEnvironment()),
	}
	for _, tool := range status.Tools {
		logToolStatus(tool)
	}
	return status
}

func logToolStatus(tool tools.Status) {
	if tool.Usable() && len(tool.Warnings) == 0 {
		diagnostics.Info("prerequisite detected", "tool", tool.Name, "version", tool.Version, "support", tool.Support)
		return
	}
	problem := tool.Error
	if problem == "" {
		problem = strings.Join(tool.Warnings, " ")
	}
	diagnostics.Warn("prerequisite unavailable or outside tested support", errors.New(problem),
		"tool", tool.Name, "installed", tool.Installed, "version", tool.Version, "support", tool.Support, "usable", tool.Usable())
}

func (application *App) SelectWorkingDirectory() (path string, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("choose project directory failed", err)
		}
	}()
	return runtime.OpenDirectoryDialog(application.ctx, runtime.OpenDialogOptions{
		Title:                      "Choose a project folder",
		CanCreateDirectories:       false,
		TreatPackagesAsDirectories: true,
	})
}

// SessionTemplate is the editable launch_args preset for a new session.
type SessionTemplate struct {
	Tool       session.Harness `json:"tool"`
	LaunchArgs []string        `json:"launchArgs"`
}

func (application *App) GetSessionTemplate(toolName string) (template SessionTemplate, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("get session template failed", err, "tool", toolName)
		}
	}()
	harnessID := session.Harness(toolName)
	adapter, err := harness.For(harnessID)
	if err != nil {
		return SessionTemplate{}, err
	}
	return SessionTemplate{Tool: harnessID, LaunchArgs: adapter.DefaultLaunchArgs()}, nil
}

// previewSessionIDPlaceholder stands in for the harness session UUID that
// TauShell only generates once a session actually starts; it is never a
// real identifier.
const previewSessionIDPlaceholder = "<new-session-id>"

// LaunchPreview is the effective executable and discrete argv TauShell would
// invoke for a fresh start with the given launch_args, shown before saving.
type LaunchPreview struct {
	Executable string   `json:"executable"`
	Arguments  []string `json:"arguments"`
	Warnings   []string `json:"warnings"`
}

func (application *App) PreviewSessionLaunch(toolName string, launchArgs []string) (preview LaunchPreview, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("preview session launch failed", err, "tool", toolName)
		}
	}()
	harnessID := session.Harness(toolName)
	adapter, err := harness.For(harnessID)
	if err != nil {
		return LaunchPreview{}, err
	}
	cleaned := make([]string, 0, len(launchArgs))
	for _, arg := range launchArgs {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if err := session.ValidateLaunchArgs(harnessID, cleaned); err != nil {
		return LaunchPreview{}, err
	}
	preview.Warnings = []string{}
	spec, _ := tools.Lookup(tools.ID(harnessID))
	executablePath, lookErr := adapter.FindExecutable()
	if lookErr != nil {
		preview.Executable = spec.Executable
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("%s was not found on PATH.", spec.Executable))
	} else {
		preview.Executable = executablePath
		if len(cleaned) > 0 {
			preview.Warnings = append(preview.Warnings, probeUnsupportedLaunchArgs(executablePath, spec, cleaned)...)
		}
	}
	preview.Arguments = adapter.Argv(cleaned, false, previewSessionIDPlaceholder)
	return preview, nil
}

// probeUnsupportedLaunchArgs reports launch_args options absent from the
// harness's own --help output. A probe failure yields a single warning
// rather than blocking the preview.
func probeUnsupportedLaunchArgs(executablePath string, spec tools.Spec, launchArgs []string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	environment := tools.DefaultEnvironment()
	help, err := environment.Run(ctx, executablePath, spec.HelpArgs...)
	if err != nil {
		return []string{fmt.Sprintf("Could not verify launch arguments: run %s %s: %v",
			filepath.Base(executablePath), strings.Join(spec.HelpArgs, " "), err)}
	}
	var warnings []string
	for _, arg := range launchArgs {
		name, _, _ := strings.Cut(arg, "=")
		if strings.HasPrefix(name, "-") && !tools.HasOption(string(help), name) {
			warnings = append(warnings, fmt.Sprintf("Option %s was not found in %s help output.", name, spec.Name))
		}
	}
	return warnings
}

func (application *App) newConfig(name, toolName, workingDir string, launchArgs []string) (session.Config, error) {
	name = strings.TrimSpace(name)
	deduced := name == ""
	if deduced {
		if strings.TrimSpace(workingDir) == "" {
			return session.Config{}, errors.New("project folder must not be empty")
		}
		name = deduceSessionName(workingDir)
	}
	id := sessionIDFromName(name)
	if id == "" {
		return session.Config{}, errors.New("session name must include at least one letter or number")
	}
	if deduced {
		// A deduced name must not collide with an existing session.
		base := name
		for n := 2; ; n++ {
			if _, statErr := os.Stat(filepath.Join(application.configDirectory, id+".yaml")); errors.Is(statErr, os.ErrNotExist) {
				break
			}
			name = fmt.Sprintf("%s (%d)", base, n)
			id = sessionIDFromName(name)
		}
	}
	return session.Config{
		ID:         id,
		Name:       name,
		Tool:       session.Harness(toolName),
		WorkingDir: workingDir,
		LaunchArgs: append([]string(nil), launchArgs...),
	}, nil
}

// PreviewSessionYAML renders the session definition the Create dialog would
// save, so it can be reviewed and edited before creation. It does not
// validate: the text is validated when saved.
func (application *App) PreviewSessionYAML(name, toolName, workingDir string, launchArgs []string) (text string, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("preview session yaml failed", err, "tool", toolName)
		}
	}()
	if strings.TrimSpace(name) == "" && strings.TrimSpace(workingDir) == "" {
		name = "new-session"
	}
	config, err := application.newConfig(name, toolName, workingDir, launchArgs)
	if err != nil {
		return "", err
	}
	return session.MarshalConfig(config)
}

// CreateSessionFromYAML saves a session from user-edited YAML after strict
// validation. It fails rather than overwrite an existing session.
func (application *App) CreateSessionFromYAML(text string) (snapshot sessionhost.Snapshot, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("create session from yaml failed", err)
			return
		}
		diagnostics.Info("session created from yaml", "session_id", snapshot.ID, "tool", snapshot.Tool)
	}()
	config, err := session.ParseConfig(text)
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	return application.saveNewConfig(config)
}

func (application *App) saveNewConfig(config session.Config) (sessionhost.Snapshot, error) {
	if _, err := session.SaveConfig(application.configDirectory, config); err != nil {
		return sessionhost.Snapshot{}, err
	}
	return sessionhost.Snapshot{
		ID:         config.ID,
		Name:       config.Name,
		Tool:       config.Tool,
		WorkingDir: config.WorkingDir,
		Status:     session.StatusNeverStarted,
	}, nil
}

func (application *App) CreateSession(name, toolName, workingDir string, launchArgs []string) (snapshot sessionhost.Snapshot, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("create session failed", err, "tool", toolName)
			return
		}
		diagnostics.Info("session created", "session_id", snapshot.ID, "tool", snapshot.Tool)
	}()
	config, err := application.newConfig(name, toolName, workingDir, launchArgs)
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	return application.saveNewConfig(config)
}

// RenameSession changes only the display name; the session ID and file name stay fixed.
func (application *App) RenameSession(id, name string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("rename session failed", err, "session_id", id)
		}
	}()
	return application.manager.UpdateDisplay(id, "name", name)
}

// SetSessionColor stores a #rrggbb accent color, or clears it when color is empty.
func (application *App) SetSessionColor(id, color string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("set session color failed", err, "session_id", id)
		}
	}()
	return application.manager.UpdateDisplay(id, "color", color)
}

// SetSessionAutoResume turns the per-session "resume on startup" flag on or off.
func (application *App) SetSessionAutoResume(id string, enabled bool) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("set session auto resume failed", err, "session_id", id)
		}
	}()
	value := ""
	if enabled {
		value = "true"
	}
	return application.manager.UpdateDisplay(id, "auto_resume", value)
}

// GetAutoResumeAll reports the global "resume all sessions on startup" preference.
func (application *App) GetAutoResumeAll() (enabled bool, err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("read auto resume all failed", err)
		}
	}()
	return application.manager.AutoResumeAll(context.Background())
}

// SetAutoResumeAll saves the global "resume all sessions on startup" preference.
func (application *App) SetAutoResumeAll(enabled bool) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("save auto resume all failed", err)
		}
	}()
	return application.manager.SetAutoResumeAll(context.Background(), enabled)
}

func (application *App) StartSession(id string, columns, rows int) (snapshot sessionhost.Snapshot, err error) {
	defer logSessionOperation("start session", id, &snapshot, &err)
	return application.manager.Start(context.Background(), id, false, columns, rows)
}

func (application *App) RestoreSession(id string, columns, rows int) (snapshot sessionhost.Snapshot, err error) {
	defer logSessionOperation("restore session", id, &snapshot, &err)
	return application.manager.Start(context.Background(), id, true, columns, rows)
}

func (application *App) StartFresh(id string, columns, rows int) (snapshot sessionhost.Snapshot, err error) {
	defer logSessionOperation("start fresh session", id, &snapshot, &err)
	return application.manager.StartFresh(context.Background(), id, columns, rows)
}

func (application *App) WriteTerminal(id, input string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("write terminal input failed", err, "session_id", id, "input_bytes", len(input))
		}
	}()
	data := []byte(input)
	if err := application.manager.Write(id, data); err != nil {
		return err
	}
	application.logTerminalReports(id, data)
	return nil
}

func (application *App) WriteTerminalBinary(id, input string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("write binary terminal input failed", err, "session_id", id)
		}
	}()
	if len(input) > 88*1024 {
		return errors.New("encoded terminal input exceeds the 88 KiB limit")
	}
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return fmt.Errorf("decode binary terminal input: %w", err)
	}
	if err := application.manager.Write(id, data); err != nil {
		return err
	}
	application.logTerminalReports(id, data)
	return nil
}

func (application *App) logTerminalReports(id string, data []byte) {
	for _, report := range terminalProtocolReports(data) {
		key := id + "\x00" + report
		if _, alreadyLogged := application.terminalReports.LoadOrStore(key, struct{}{}); alreadyLogged {
			continue
		}
		diagnostics.Debug("terminal protocol input forwarded",
			"session_id", id,
			"protocol", report,
		)
	}
}

func terminalProtocolReports(data []byte) []string {
	var reports []string
	seen := make(map[string]struct{}, 2)
	add := func(report string) {
		if _, exists := seen[report]; exists {
			return
		}
		seen[report] = struct{}{}
		reports = append(reports, report)
	}

	for offset := 0; offset+2 < len(data); offset++ {
		if data[offset] != 0x1b || data[offset+1] != '[' {
			continue
		}
		start := offset + 2
		if data[start] == 'M' && len(data)-start >= 4 {
			add("mouse-x10")
			continue
		}
		if data[start] == '<' {
			for end := start + 1; end < len(data); end++ {
				if data[end] == 'M' || data[end] == 'm' {
					add("mouse-sgr")
					break
				}
				if (data[end] < '0' || data[end] > '9') && data[end] != ';' {
					break
				}
			}
			continue
		}

		end := start
		for end < len(data) && ((data[end] >= '0' && data[end] <= '9') || data[end] == ';') {
			end++
		}
		if end >= len(data) {
			continue
		}
		switch {
		case data[end] == 'n' && bytes.Equal(data[start:end], []byte("0")):
			add("device-status")
		case data[end] == 'R' && bytes.IndexByte(data[start:end], ';') >= 0:
			add("cursor-position")
		}
	}
	return reports
}

func (application *App) ResizeTerminal(id string, columns, rows int) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("resize terminal failed", err, "session_id", id, "columns", columns, "rows", rows)
		}
	}()
	return application.manager.Resize(id, columns, rows)
}

func (application *App) DeleteSession(id string) (err error) {
	defer func() {
		if err != nil {
			diagnostics.Error("delete session failed", err, "session_id", id)
		} else {
			diagnostics.Info("session deleted", "session_id", id)
		}
	}()
	return application.manager.Delete(context.Background(), id)
}

func (application *App) StopSession(id string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	defer func() {
		if err != nil {
			diagnostics.Error("stop session failed", err, "session_id", id)
		} else {
			diagnostics.Info("session stop requested", "session_id", id)
		}
	}()
	return application.manager.Stop(ctx, id)
}

func (application *App) LogFrontendFailure(source, message, stack string) {
	diagnostics.FrontendFailure(source, message, stack)
}

func (application *App) LogTerminalPointerEvent(id, event string) {
	if event != "pointerdown" && event != "mousedown" {
		diagnostics.Warn("unsupported terminal pointer event reported", errors.New("invalid event type"), "session_id", id)
		return
	}
	key := id + "\x00" + event
	if _, alreadyLogged := application.terminalReports.LoadOrStore(key, struct{}{}); alreadyLogged {
		return
	}
	diagnostics.Debug("terminal pointer event received", "session_id", id, "event", event)
}

func (application *App) LogTerminalMouseTrackingMode(id, mode string) {
	switch mode {
	case "none", "x10", "vt200", "drag", "any":
	default:
		diagnostics.Warn("unsupported terminal mouse tracking mode reported", errors.New("invalid mode"), "session_id", id)
		return
	}
	key := id + "\x00mouse-mode\x00" + mode
	if _, alreadyLogged := application.terminalReports.LoadOrStore(key, struct{}{}); alreadyLogged {
		return
	}
	diagnostics.Debug("terminal mouse tracking mode changed", "session_id", id, "mode", mode)
}

func logSessionOperation(operation, id string, snapshot *sessionhost.Snapshot, err *error) {
	if *err != nil {
		diagnostics.Error(operation+" failed", *err, "session_id", id)
		return
	}
	diagnostics.Info(operation+" completed", "session_id", id, "status", snapshot.Status, "running_here", snapshot.RunningHere)
}

func (application *App) beforeClose(ctx context.Context) bool {
	application.shutdownMu.Lock()
	if application.allowShutdown {
		application.allowShutdown = false
		application.shutdownMu.Unlock()
		application.saveWindowPlacement()
		return false
	}
	if application.shutdownInProgress {
		application.shutdownMu.Unlock()
		return true
	}
	application.shutdownInProgress = true
	application.shutdownMu.Unlock()

	go application.confirmShutdownAsync(ctx)
	return true
}

func (application *App) confirmShutdownAsync(ctx context.Context) {
	allowShutdown := false
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("shutdown confirmation panicked", fmt.Errorf("%v", recovered), "stack", string(debug.Stack()))
			allowShutdown = false
		}
		application.shutdownMu.Lock()
		application.shutdownInProgress = false
		if allowShutdown {
			application.allowShutdown = true
		}
		application.shutdownMu.Unlock()
		if allowShutdown {
			diagnostics.Info("shutdown confirmed; requesting application close")
			application.requestApplicationQuit(ctx)
		}
	}()

	allowShutdown = !application.confirmShutdown(ctx)
}

func (application *App) requestApplicationQuit(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("request application shutdown panicked", fmt.Errorf("%v", recovered), "stack", string(debug.Stack()))
		}
	}()
	if application.requestQuit != nil {
		application.requestQuit(ctx)
		return
	}
	runtime.Quit(ctx)
}

func (application *App) confirmShutdown(ctx context.Context) bool {
	sessions, err := application.manager.List(ctx)
	if err != nil {
		diagnostics.Error("check active sessions before shutdown failed", err)
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Unable to check active sessions",
			Message: err.Error(),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
			diagnostics.Error("report active-session check failure failed", dialogErr)
			fmt.Fprintf(os.Stderr, "taushell: report active sessions check failure: %v\n", dialogErr)
		}
		return true
	}
	active := make([]string, 0)
	for _, current := range sessions {
		if current.RunningHere {
			active = append(active, current.Name)
		}
	}
	if len(active) == 0 {
		diagnostics.Info("shutdown requested with no locally running sessions")
		return application.reportForcedShutdowns(ctx, nil)
	}
	diagnostics.Info("shutdown confirmation required", "active_session_count", len(active))
	choice, err := application.messageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Sessions are still running",
		Message:       fmt.Sprintf("%d session(s) are running in TauShell. Closing will interrupt them. Stop the sessions and close TauShell?", len(active)),
		Buttons:       []string{"Yes", "No"},
		DefaultButton: "No",
		CancelButton:  "No",
	})
	if err != nil {
		diagnostics.Error("show shutdown confirmation failed", err)
		fmt.Fprintf(os.Stderr, "taushell: confirm active session shutdown: %v\n", err)
		return true
	}
	if !strings.EqualFold(choice, "Yes") {
		diagnostics.Info("shutdown cancelled by user")
		return true
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	finishProgress := application.trackShutdownProgress(active)
	defer finishProgress()
	if err := application.manager.StopAll(stopCtx); err != nil {
		diagnostics.Error("stop sessions for shutdown failed", err)
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Unable to stop all sessions",
			Message: err.Error(),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
			diagnostics.Error("report session shutdown failure failed", dialogErr)
			fmt.Fprintf(os.Stderr, "taushell: report session shutdown failure: %v\n", dialogErr)
		}
		return true
	}
	stopped, err := application.manager.List(ctx)
	if err != nil {
		diagnostics.Error("verify stopped sessions failed", err)
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Unable to verify stopped sessions",
			Message: err.Error(),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
			diagnostics.Error("report stopped-session verification failure failed", dialogErr)
			fmt.Fprintf(os.Stderr, "taushell: report stopped-session verification failure: %v\n", dialogErr)
		}
		return true
	}
	forced := make([]string, 0)
	stillRunning := make([]string, 0)
	for _, current := range stopped {
		if current.RunningHere {
			stillRunning = append(stillRunning, current.Name)
		} else if strings.Contains(current.LastError, "force-terminated") {
			forced = append(forced, current.Name)
		}
	}
	if len(stillRunning) > 0 {
		diagnostics.Error("sessions remain running after shutdown", errors.New("some session processes remain active"), "count", len(stillRunning))
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Some sessions are still running",
			Message: fmt.Sprintf("TauShell could not verify that these sessions stopped:\n\n%s", strings.Join(stillRunning, "\n")),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
			diagnostics.Error("report sessions still running failed", dialogErr)
			fmt.Fprintf(os.Stderr, "taushell: report sessions still running after shutdown: %v\n", dialogErr)
		}
		return true
	}
	return application.reportForcedShutdowns(ctx, forced)
}

func (application *App) messageDialog(ctx context.Context, options runtime.MessageDialogOptions) (string, error) {
	if application.showMessageDialog != nil {
		return application.showMessageDialog(ctx, options)
	}
	if choice, shown, err := application.inAppDialog(ctx, options); shown {
		return choice, err
	}
	// The page is not up (or has not registered), so fall back to the native box.
	return runtime.MessageDialog(ctx, options)
}

func (application *App) reportForcedShutdowns(ctx context.Context, names []string) bool {
	application.forcedShutdownWarnings = append(application.forcedShutdownWarnings, names...)
	if len(application.forcedShutdownWarnings) == 0 {
		return false
	}
	diagnostics.Warn("sessions required force termination", errors.New("graceful interruption did not stop all session processes"), "count", len(application.forcedShutdownWarnings))
	_, err := application.messageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.WarningDialog,
		Title:   "Some sessions required force termination",
		Message: fmt.Sprintf("TauShell force-terminated these session process trees because graceful interruption did not stop them:\n\n%s", strings.Join(application.forcedShutdownWarnings, "\n")),
		Buttons: []string{"Continue closing"},
	})
	if err != nil {
		diagnostics.Error("report forced session shutdown failed", err)
		fmt.Fprintf(os.Stderr, "taushell: report forced session shutdown: %v\n", err)
		return true
	}
	application.forcedShutdownWarnings = nil
	return false
}

func (application *App) shutdown(ctx context.Context) {
	stopCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := application.manager.Close(stopCtx); err != nil {
		diagnostics.Error("close session manager during Wails shutdown failed", err)
		fmt.Fprintf(os.Stderr, "taushell: close session manager: %v\n", err)
		return
	}
	diagnostics.Info("session manager closed")
}

func installedWebView2Version() (string, error) {
	const clients = `SOFTWARE\Microsoft\EdgeUpdate\Clients`
	for _, hive := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			root, err := registry.OpenKey(hive, clients, registry.READ|view)
			if err != nil {
				if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
					continue
				}
				return "", err
			}
			names, enumErr := root.ReadSubKeyNames(-1)
			closeErr := root.Close()
			if enumErr != nil || closeErr != nil {
				return "", errors.Join(enumErr, closeErr)
			}
			for _, name := range names {
				key, err := registry.OpenKey(hive, filepath.Join(clients, name), registry.READ|view)
				if err != nil {
					if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
						continue
					}
					return "", err
				}
				product, _, productErr := key.GetStringValue("name")
				version, _, versionErr := key.GetStringValue("pv")
				closeErr := key.Close()
				if closeErr != nil {
					return "", closeErr
				}
				if productErr == nil && versionErr == nil && strings.EqualFold(product, "Microsoft Edge WebView2 Runtime") && version != "" {
					return version, nil
				}
			}
		}
	}
	return "", nil
}

func explainMissingWebView2() error {
	message, err := windows.UTF16PtrFromString("TauShell requires the Microsoft Edge WebView2 Runtime to open its desktop interface.\n\nOpen the official Microsoft download page?")
	if err != nil {
		return err
	}
	title, err := windows.UTF16PtrFromString("WebView2 Runtime required")
	if err != nil {
		return err
	}
	result, err := windows.MessageBox(0, message, title, windows.MB_ICONWARNING|windows.MB_YESNO)
	if err != nil {
		return fmt.Errorf("show WebView2 requirement: %w", err)
	}
	if result != 6 {
		return nil
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	url, err := windows.UTF16PtrFromString("https://developer.microsoft.com/microsoft-edge/webview2/")
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, verb, url, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("open Microsoft WebView2 download page: %w", err)
	}
	return errors.New("install Microsoft Edge WebView2 Runtime, then launch TauShell again")
}
