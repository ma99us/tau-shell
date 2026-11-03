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

	"github.com/ma99us/tau-shell/internal/conpty"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/terminal"
	"github.com/ma99us/tau-shell/internal/tools"
	"github.com/ma99us/tau-shell/internal/vault"
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
	if layout, err := tools.ManagedToolLayoutFromEnvironment(); err != nil {
		diagnostics.Warn("managed tool paths unavailable", err)
	} else if err := layout.PrependToProcessPath(); err != nil {
		diagnostics.Warn("configure managed tool PATH failed", err)
	}

	command := "desktop"
	loadHarnessManifests()
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	diagnostics.Info("application process started", "pid", os.Getpid(), "command", command)
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		diagnostics.Error("application command failed", err, "command", command)
		fmt.Fprintln(os.Stderr, "taushell:", err)
		var coded *ctlError
		if errors.As(err, &coded) {
			return coded.code
		}
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
	case "ctl":
		return runCtl(args[1:], stdout)
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
  taushell ctl list [--json] [--instance <pid>]
  taushell ctl wait <session> [--for idle|input|working] [--timeout 10m]
  taushell ctl send [--insert-only] [--stdin] <session> <text...>
  taushell ctl start [--restore|--fresh] <session>
  taushell ctl stop <session>
  taushell ctl read [--rows N] <session>

Session definitions: <user-home>\.tau_shell\sessions\<session-id>.yaml
No-argument launch opens the TauShell desktop application.
The list, start, and restore commands remain available without the desktop UI.
ctl talks to a running TauShell and needs control.enabled: true in config.yaml.
ctl exit codes: 0 ok, 1 error, 2 usage, 3 TauShell not running or session not found, 4 channel disabled, 5 refused, 6 wait timed out.`)
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
	store, err := openStore(databasePath)
	if err != nil {
		return nil, err
	}
	if err := session.RecoverPendingConfigDeletions(configDirectory, store); err != nil {
		return nil, errors.Join(err, store.Close())
	}
	configs, err := session.ListConfigs(configDirectory)
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	if len(configs) == 0 {
		return nil, store.Close()
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].ID < configs[j].ID })
	entries := make([]sessionEntry, 0, len(configs))
	for _, config := range configs {
		runtime, err := session.RecoverRuntime(context.Background(), store, config.ID)
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
	store, err := openStore(databasePath)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, store.Close())
	}()
	if err := session.RecoverPendingConfigDeletions(configDirectory, store); err != nil {
		return err
	}
	unlockConfig, err := session.LockConfig(configDirectory, id)
	if err != nil {
		return fmt.Errorf("lock session config %q: %w", id, err)
	}
	defer unlockConfig()
	config, err := session.FindConfig(configDirectory, id)
	if err != nil {
		return err
	}
	adapter, err := harness.For(config.Tool)
	if err != nil {
		return err
	}
	if err := manifest.Allowed(string(config.Tool)); err != nil {
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
	launchEnv, err := config.LaunchEnvironment(vault.NewCredentialManager())
	if err != nil {
		return err
	}
	runtime, err := session.RecoverRuntime(context.Background(), store, id)
	if err != nil {
		return err
	}
	if runtime.Status == session.StatusRunning || runtime.Status == session.StatusStarting {
		return fmt.Errorf("session %q is already active in another TauShell process", id)
	}
	fresh := false
	if restoring && runtime.HarnessSessionID != "" {
		if resumeErr := adapter.CheckResume(runtime.HarnessSessionID); resumeErr != nil {
			// Same as the desktop app: warn, then start a new conversation.
			fmt.Fprintf(os.Stderr, "Could not resume: %v. Starting a new conversation instead.\n", resumeErr)
			restoring, fresh = false, true
		}
	}
	if fresh {
		if adapter.DiscoversSessionID() {
			runtime, err = store.BeginDiscovered(context.Background(), id, config.Tool, false, true)
		} else {
			runtime, err = store.BeginFresh(context.Background(), id, config.Tool)
		}
	} else if adapter.DiscoversSessionID() {
		runtime, err = store.BeginDiscovered(context.Background(), id, config.Tool, restoring, false)
	} else {
		runtime, err = store.Begin(context.Background(), id, config.Tool, restoring)
	}
	if err != nil {
		return err
	}
	unlockConfig()
	var existingSessionIDs map[string]bool
	discoveryNote := ""
	if adapter.DiscoversSessionID() && runtime.HarnessSessionID == "" {
		existingSessionIDs, err = adapter.ExistingSessionIDs(config.WorkingDir)
		if err != nil {
			discoveryNote = "Resume ID unknown: existing " + string(config.Tool) + " conversations could not be enumerated; use Start fresh."
			diagnostics.Warn("snapshot existing harness sessions failed", err, "session_id", id, "tool", config.Tool)
		}
	}
	launchedAt := time.Now()
	args := adapter.Argv(config.LaunchArgs, restoring, runtime.HarnessSessionID)
	started, finished := false, false
	stopDiscovery := make(chan struct{})
	discoveryDone := make(chan string, 1)
	if adapter.DiscoversSessionID() && runtime.HarnessSessionID == "" && discoveryNote == "" {
		go func() {
			discoveryDone <- discoverCLISessionID(store, adapter, config, launchedAt, existingSessionIDs, runtime.LaunchToken, stopDiscovery)
		}()
	} else {
		discoveryDone <- discoveryNote
		close(discoveryDone)
	}
	defer func() {
		close(stopDiscovery)
		if note, ok := <-discoveryDone; ok && note != "" {
			fmt.Fprintln(os.Stderr, note)
		}
	}()
	runErr := terminal.Run(terminal.Options{
		Executable: powershell,
		Directory:  config.WorkingDir,
		Args: []string{
			"-NoLogo", "-NoProfile", "-Command",
			harness.PowerShellInvocation(executable, args, config.UTF8Enabled()),
		},
		Env: conpty.ChildEnvironment(launchEnv),
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

// discoverCLISessionID is the foreground-CLI counterpart of the desktop
// manager's discovery: it polls until the ID is recorded or stop closes, with
// one last attempt after stop.
func discoverCLISessionID(store *session.Store, adapter harness.Adapter, config session.Config, launchedAt time.Time, existingIDs map[string]bool, launchToken string, stop <-chan struct{}) string {
	lastNote := ""
	attempt := func() bool {
		ctx := context.Background()
		found, err := harness.DiscoverAndClaimSessionID(ctx, store, adapter, config.ID, config.WorkingDir, launchedAt, existingIDs, launchToken)
		if errors.Is(err, harness.ErrAmbiguousSession) {
			lastNote = "Resume ID unknown: several " + string(config.Tool) + " conversations in this folder match; use Start fresh."
			return false
		}
		if errors.Is(err, session.ErrHarnessSessionIDClaimed) {
			lastNote = "Resume ID unknown: the discovered conversation is already linked to another TauShell session."
			return true
		}
		if errors.Is(err, session.ErrHarnessLaunchChanged) {
			lastNote = "Resume ID discovery result discarded because this session has since started again."
			return true
		}
		if err != nil {
			lastNote = "Resume ID discovery failed: " + err.Error()
			return false
		}
		if found == "" {
			return false
		}
		lastNote = ""
		return true
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			attempt()
			return lastNote
		case <-ticker.C:
			if attempt() {
				return lastNote
			}
		}
	}
}
