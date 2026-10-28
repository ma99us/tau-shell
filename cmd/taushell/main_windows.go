package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/terminal"
	"golang.org/x/sys/windows"
)

func main() {
	os.Exit(runMain())
}

func runMain() (exitCode int) {
	closeLog, err := diagnostics.Init()
	if err != nil {
		message := fmt.Sprintf("TauShell could not create its diagnostic log at %s: %v", diagnostics.LogPath(), err)
		if len(os.Args) == 1 {
			title, titleErr := windows.UTF16PtrFromString("TauShell startup failed")
			body, bodyErr := windows.UTF16PtrFromString(message)
			if titleErr == nil && bodyErr == nil {
				_, _ = windows.MessageBox(0, body, title, windows.MB_ICONERROR|windows.MB_OK)
				return 1
			}
		}
		fmt.Fprintln(os.Stderr, "taushell:", message)
		return 1
	}
	defer func() {
		if err := closeLog(); err != nil {
			fmt.Fprintln(os.Stderr, "taushell: close diagnostic log:", err)
		}
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("unhandled application panic", fmt.Errorf("%v", recovered), "stack", string(debug.Stack()))
			exitCode = 2
		}
	}()

	command := "desktop"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	diagnostics.Info("application process started", "pid", os.Getpid(), "command", command)
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		diagnostics.Error("application command failed", err, "command", command)
		fmt.Fprintln(os.Stderr, "taushell:", err)
		return 1
	}
	diagnostics.Info("application process completed", "command", command)
	return 0
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || isToastActivationArg(args) {
		return runDesktop()
	}
	return terminal.WithUTF8Console(func() error {
		return runCommand(args, stdout, stderr)
	})
}

func runCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return runDesktop()
	}
	switch args[0] {
	case "help", "--help", "-h":
		printUsage(stdout)
		return nil
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("list does not accept arguments")
		}
		return listSessions(stdout)
	case "start", "restore":
		if len(args) != 2 {
			return fmt.Errorf("%s requires one session ID", args[0])
		}
		sessionID, restoring := sessionCommandArgs(args[0], args[1])
		return runSession(sessionID, restoring)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func sessionCommandArgs(command, sessionID string) (string, bool) {
	return sessionID, command == "restore"
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `TauShell - local PowerShell AI session manager

Usage:
  taushell
  taushell list
  taushell start <session-id>
  taushell restore <session-id>

Session definitions: <user-home>\.tau_shell\sessions\<session-id>.yaml
No-argument launch opens the TauShell desktop application.
The list, start, and restore commands remain available without the desktop UI.`)
}

func appPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("find user home directory: %w", err)
	}
	root := filepath.Join(home, ".tau_shell")
	return filepath.Join(root, "sessions"), filepath.Join(root, "state.db"), nil
}

func openStore(databasePath string) (*session.Store, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create TauShell data directory: %w", err)
	}
	store, err := session.OpenStore(databasePath)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func listSessions(writer io.Writer) (returnErr error) {
	configDirectory, databasePath, err := appPaths()
	if err != nil {
		return err
	}
	return listSessionsAt(configDirectory, databasePath, writer)
}

func listSessionsAt(configDirectory, databasePath string, writer io.Writer) (returnErr error) {
	entries, err := loadSessionEntries(configDirectory, databasePath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintf(writer, "No sessions configured. Add per-session YAML files to %s\n", configDirectory)
		return nil
	}
	fmt.Fprintf(writer, "%-24s %-16s %-12s %s\n", "SESSION", "TOOL", "STATUS", "NAME")
	for _, entry := range entries {
		fmt.Fprintf(writer, "%-24s %-16s %-12s %s\n", entry.Config.ID, entry.Config.Tool, entry.Runtime.Status, entry.Config.Name)
		if entry.Runtime.LastError != "" {
			fmt.Fprintf(writer, "  Last error: %s\n", entry.Runtime.LastError)
		}
	}
	return nil
}

type sessionEntry struct {
	Config  session.Config
	Runtime session.Runtime
}

func loadSessionEntries(configDirectory, databasePath string) ([]sessionEntry, error) {
	configs, err := session.ListConfigs(configDirectory)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		return nil, nil
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].ID < configs[j].ID })
	store, err := openStore(databasePath)
	if err != nil {
		return nil, err
	}
	entries := make([]sessionEntry, 0, len(configs))
	for _, config := range configs {
		runtime, err := recoverRuntime(context.Background(), store, config.ID)
		if err != nil {
			return nil, errors.Join(err, store.Close())
		}
		entries = append(entries, sessionEntry{Config: config, Runtime: runtime})
	}
	if err := store.Close(); err != nil {
		return nil, err
	}
	return entries, nil
}

func runSession(id string, restoring bool) (returnErr error) {
	configDirectory, databasePath, err := appPaths()
	if err != nil {
		return err
	}
	config, err := session.FindConfig(configDirectory, id)
	if err != nil {
		return err
	}
	adapter, err := harness.For(config.Tool)
	if err != nil {
		return err
	}
	executable, err := adapter.FindExecutable()
	if err != nil {
		return err
	}
	powershell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		return fmt.Errorf("PowerShell 7 (pwsh.exe) is required and must be on PATH: %w", err)
	}
	store, err := openStore(databasePath)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, store.Close())
	}()
	runtime, err := recoverRuntime(context.Background(), store, id)
	if err != nil {
		return err
	}
	if runtime.Status == session.StatusRunning || runtime.Status == session.StatusStarting {
		return fmt.Errorf("session %q is already active in another TauShell process", id)
	}
	if adapter.DiscoversSessionID() {
		runtime, err = store.BeginDiscovered(context.Background(), id, config.Tool, restoring, false)
	} else {
		runtime, err = store.Begin(context.Background(), id, config.Tool, restoring)
	}
	if err != nil {
		return err
	}
	launchedAt := time.Now()
	args := adapter.Argv(config.LaunchArgs, restoring, runtime.HarnessSessionID)
	started, finished := false, false
	stopDiscovery := make(chan struct{})
	discoveryDone := make(chan struct{})
	if adapter.DiscoversSessionID() && runtime.HarnessSessionID == "" {
		go func() {
			defer close(discoveryDone)
			discoverCLISessionID(store, adapter, config, launchedAt, stopDiscovery)
		}()
	} else {
		close(discoveryDone)
	}
	defer func() {
		close(stopDiscovery)
		<-discoveryDone
	}()
	runErr := terminal.Run(terminal.Options{
		Executable: powershell,
		Directory:  config.WorkingDir,
		Args: []string{
			"-NoLogo", "-NoProfile", "-Command",
			harness.PowerShellInvocation(executable, args, config.Environment, config.UTF8Enabled()),
		},
		OnStart: func(processID int) error {
			if err := store.MarkRunning(context.Background(), id, processID); err != nil {
				return err
			}
			started = true
			return nil
		},
		OnFinish: func(status, detail string) error {
			finished = true
			return store.Finish(context.Background(), id, session.RuntimeStatus(status), detail)
		},
	})
	if runErr != nil && !finished {
		return errors.Join(runErr, store.Finish(context.Background(), id, session.StatusFailed, runErr.Error()))
	}
	if !started && runErr == nil {
		return fmt.Errorf("session %q exited without starting", id)
	}
	return runErr
}

const startupRecoveryDelay = time.Minute

func recoverRuntime(ctx context.Context, store *session.Store, id string) (session.Runtime, error) {
	for attempt := 0; attempt < 3; attempt++ {
		runtime, err := store.Runtime(ctx, id)
		if err != nil {
			return session.Runtime{}, err
		}
		switch runtime.Status {
		case session.StatusStarting:
			if time.Since(runtime.UpdatedAt) < startupRecoveryDelay {
				return runtime, nil
			}
		case session.StatusRunning:
			alive, err := session.RuntimeProcessAlive(runtime)
			if err != nil {
				return session.Runtime{}, err
			}
			if alive {
				return runtime, nil
			}
		default:
			return runtime, nil
		}

		detail := "TauShell exited unexpectedly; use restore to resume the saved harness session."
		if runtime.Status == session.StatusStarting {
			detail = "TauShell stopped while starting the session; use start or restore to retry."
		}
		updated, err := store.FailIfCurrent(ctx, runtime, detail)
		if err != nil {
			return session.Runtime{}, err
		}
		if updated {
			runtime.Status = session.StatusFailed
			runtime.ProcessID = 0
			runtime.LastError = detail
			return runtime, nil
		}
	}
	return session.Runtime{}, fmt.Errorf("session %q runtime state changed repeatedly during recovery; retry the command", id)
}

// discoverCLISessionID is the foreground-CLI counterpart of the desktop
// manager's discovery: it polls until the ID is recorded or stop closes, with
// one last attempt after stop.
func discoverCLISessionID(store *session.Store, adapter harness.Adapter, config session.Config, launchedAt time.Time, stop <-chan struct{}) {
	attempt := func() bool {
		ctx := context.Background()
		taken, err := store.HarnessSessionIDs(ctx, config.Tool)
		if err != nil {
			return false
		}
		found, err := adapter.DiscoverSessionID(config.WorkingDir, launchedAt, taken)
		if err != nil || found == "" {
			return false
		}
		recorded, err := store.SetHarnessSessionID(ctx, config.ID, found)
		return err == nil && recorded
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			attempt()
			return
		case <-ticker.C:
			if attempt() {
				return
			}
		}
	}
}
