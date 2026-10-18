//go:build windows

package main

import (
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
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/sessionhost"
	"github.com/wailsapp/wails/v2"
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
	forcedShutdownWarnings []string
}

type desktopSessionManager interface {
	List(context.Context) ([]sessionhost.Snapshot, error)
	Start(context.Context, string, bool, int, int) (sessionhost.Snapshot, error)
	StartFresh(context.Context, string, int, int) (sessionhost.Snapshot, error)
	Write(string, []byte) error
	Resize(string, int, int) error
	Stop(context.Context, string) error
	StopAll(context.Context) error
	Close(context.Context) error
}

type ToolStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

type SystemStatus struct {
	WebView2Version string     `json:"webView2Version"`
	PowerShell      ToolStatus `json:"powershell"`
	ClaudeCode      ToolStatus `json:"claudeCode"`
	CopilotCLI      ToolStatus `json:"copilotCLI"`
}

func runDesktop() error {
	version, err := installedWebView2Version()
	if err != nil {
		return fmt.Errorf("check Microsoft Edge WebView2 Runtime: %w", err)
	}
	if version == "" {
		return explainMissingWebView2()
	}

	configDirectory, databasePath, err := appPaths()
	if err != nil {
		return err
	}
	assetFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("load embedded desktop assets: %w", err)
	}
	application := &App{configDirectory: configDirectory, webViewVersion: version}
	application.manager, err = sessionhost.New(configDirectory, databasePath, application.publish)
	if err != nil {
		return fmt.Errorf("initialize session manager: %w", err)
	}
	runErr := wails.Run(&options.App{
		Title:       "TauShell",
		Width:       1180,
		Height:      780,
		MinWidth:    760,
		MinHeight:   520,
		AssetServer: &assetserver.Options{Assets: assetFS},
		Bind:        []interface{}{application},
		OnNavigationStarting: func(_ context.Context, targetURL string) bool {
			if isBundledAppNavigation(targetURL) {
				return true
			}
			fmt.Fprintln(os.Stderr, "taushell: blocked WebView navigation outside the bundled app origin")
			return false
		},
		OnStartup:     application.startup,
		OnBeforeClose: application.beforeClose,
		OnShutdown:    application.shutdown,
		Windows: &windowsOptions.Options{
			Theme: windowsOptions.Dark,
		},
	})
	closeCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	return errors.Join(runErr, application.manager.Close(closeCtx))
}

func (application *App) startup(ctx context.Context) {
	application.ctx = ctx
}

func (application *App) publish(event sessionhost.Event) {
	if application.ctx != nil {
		runtime.EventsEmit(application.ctx, "session-event", event)
	}
}

func (application *App) GetSessions() ([]sessionhost.Snapshot, error) {
	return application.manager.List(context.Background())
}

func (application *App) GetSystemStatus() SystemStatus {
	return SystemStatus{
		WebView2Version: application.webViewVersion,
		PowerShell:      detectTool("PowerShell 7", "pwsh.exe", "--version"),
		ClaudeCode:      detectHarness(session.ClaudeCode),
		CopilotCLI:      detectHarness(session.CopilotCLI),
	}
}

func detectHarness(tool session.Harness) ToolStatus {
	status := ToolStatus{Name: string(tool)}
	adapter, err := harness.For(tool)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	executable, err := adapter.FindExecutable()
	if err != nil {
		status.Error = err.Error()
		return status
	}
	return detectTool(status.Name, executable, "--version")
}

func detectTool(name, executable, versionFlag string) ToolStatus {
	status := ToolStatus{Name: name}
	path, err := exec.LookPath(executable)
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.Installed = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, versionFlag).CombinedOutput()
	if err != nil {
		status.Error = fmt.Sprintf("run %s %s: %v", filepath.Base(path), versionFlag, err)
		status.Version = "Version check failed"
		return status
	}
	version := strings.TrimSpace(string(output))
	if len(version) > 200 {
		version = version[:200]
	}
	status.Version = version
	return status
}

func (application *App) SelectWorkingDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(application.ctx, runtime.OpenDialogOptions{
		Title:                      "Choose a project folder",
		CanCreateDirectories:       false,
		TreatPackagesAsDirectories: true,
	})
}

func (application *App) CreateSession(name, toolName, workingDir string, launchArgs []string) (sessionhost.Snapshot, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return sessionhost.Snapshot{}, errors.New("session name must not be empty")
	}
	id := strings.Trim(sessionIDPattern.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(id) > 63 {
		id = strings.TrimRight(id[:63], "-")
	}
	if id == "" {
		return sessionhost.Snapshot{}, errors.New("session name must include at least one letter or number")
	}
	config := session.Config{
		ID:         id,
		Name:       name,
		Tool:       session.Harness(toolName),
		WorkingDir: workingDir,
		LaunchArgs: append([]string(nil), launchArgs...),
	}
	if _, err := session.SaveConfig(application.configDirectory, config); err != nil {
		return sessionhost.Snapshot{}, err
	}
	sessions, err := application.manager.List(context.Background())
	if err != nil {
		return sessionhost.Snapshot{}, err
	}
	for _, current := range sessions {
		if current.ID == id {
			return current, nil
		}
	}
	return sessionhost.Snapshot{}, fmt.Errorf("session %q was saved but is missing from the session list", id)
}

func (application *App) StartSession(id string, columns, rows int) (sessionhost.Snapshot, error) {
	return application.manager.Start(context.Background(), id, false, columns, rows)
}

func (application *App) RestoreSession(id string, columns, rows int) (sessionhost.Snapshot, error) {
	return application.manager.Start(context.Background(), id, true, columns, rows)
}

func (application *App) StartFresh(id string, columns, rows int) (sessionhost.Snapshot, error) {
	return application.manager.StartFresh(context.Background(), id, columns, rows)
}

func (application *App) WriteTerminal(id, input string) error {
	return application.manager.Write(id, []byte(input))
}

func (application *App) WriteTerminalBinary(id, input string) error {
	if len(input) > 88*1024 {
		return errors.New("encoded terminal input exceeds the 88 KiB limit")
	}
	data, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return fmt.Errorf("decode binary terminal input: %w", err)
	}
	return application.manager.Write(id, data)
}

func (application *App) ResizeTerminal(id string, columns, rows int) error {
	return application.manager.Resize(id, columns, rows)
}

func (application *App) StopSession(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return application.manager.Stop(ctx, id)
}

func (application *App) beforeClose(ctx context.Context) bool {
	sessions, err := application.manager.List(ctx)
	if err != nil {
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Unable to check active sessions",
			Message: err.Error(),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
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
		return application.reportForcedShutdowns(ctx, nil)
	}
	choice, err := application.messageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Sessions are still running",
		Message:       fmt.Sprintf("%d session(s) are running in TauShell. Closing will interrupt them. Stop the sessions and close TauShell?", len(active)),
		Buttons:       []string{"Stop sessions and close", "Keep TauShell open"},
		DefaultButton: "Keep TauShell open",
		CancelButton:  "Keep TauShell open",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "taushell: confirm active session shutdown: %v\n", err)
		return true
	}
	if choice != "Stop sessions and close" {
		return true
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := application.manager.StopAll(stopCtx); err != nil {
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Unable to stop all sessions",
			Message: err.Error(),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
			fmt.Fprintf(os.Stderr, "taushell: report session shutdown failure: %v\n", dialogErr)
		}
		return true
	}
	stopped, err := application.manager.List(ctx)
	if err != nil {
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Unable to verify stopped sessions",
			Message: err.Error(),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
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
		_, dialogErr := application.messageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "Some sessions are still running",
			Message: fmt.Sprintf("TauShell could not verify that these sessions stopped:\n\n%s", strings.Join(stillRunning, "\n")),
			Buttons: []string{"Keep TauShell open"},
		})
		if dialogErr != nil {
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
	return runtime.MessageDialog(ctx, options)
}

func (application *App) reportForcedShutdowns(ctx context.Context, names []string) bool {
	application.forcedShutdownWarnings = append(application.forcedShutdownWarnings, names...)
	if len(application.forcedShutdownWarnings) == 0 {
		return false
	}
	_, err := application.messageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.WarningDialog,
		Title:   "Some sessions required force termination",
		Message: fmt.Sprintf("TauShell force-terminated these session process trees because graceful interruption did not stop them:\n\n%s", strings.Join(application.forcedShutdownWarnings, "\n")),
		Buttons: []string{"Continue closing"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "taushell: report forced session shutdown: %v\n", err)
		return true
	}
	application.forcedShutdownWarnings = nil
	return false
}

func (application *App) shutdown(ctx context.Context) {
	stopCtx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := application.manager.Close(stopCtx); err != nil {
		fmt.Fprintf(os.Stderr, "taushell: close session manager: %v\n", err)
	}
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
