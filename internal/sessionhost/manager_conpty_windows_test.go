package sessionhost

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/conpty"
	"github.com/ma99us/tau-shell/internal/session"
)

// useRealPowerShell runs sessions in a real ConPTY with script as the harness stand-in.
func useRealPowerShell(t *testing.T, manager *Manager, realPwsh, script string) {
	t.Helper()
	manager.startProcess = func(columns, rows int, _, directory string, env []string, _ ...string) (sessionProcess, error) {
		process, _, err := conpty.StartPreferredEnv(conpty.ExecutableDirectory(), env, columns, rows, realPwsh, directory,
			"-NoLogo", "-NoProfile", "-Command", script)
		if err != nil {
			return nil, err
		}
		return process, nil
	}
}

func lookRealPowerShell(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("PowerShell 7 is not installed")
	}
	return path
}

func TestRealSessionStopReportsForcedTerminationWhenInterruptIgnored(t *testing.T) {
	realPwsh := lookRealPowerShell(t)
	manager, _, _ := newLifecycleTestManager(t, "real-forced")
	useRealPowerShell(t, manager, realPwsh, "[Console]::TreatControlCAsInput = $true; while ($true) { Start-Sleep -Milliseconds 200 }")
	ctx := context.Background()
	started, err := manager.Start(ctx, "real-forced", false, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != session.StatusRunning {
		t.Fatalf("status after start = %q", started.Status)
	}
	time.Sleep(time.Second)
	if err := manager.Stop(ctx, "real-forced"); err != nil {
		t.Fatal(err)
	}
	snapshot := manager.sessions["real-forced"].snapshot()
	if snapshot.Status != session.StatusStopped || !strings.Contains(snapshot.LastError, "force-terminated") {
		t.Fatalf("snapshot = %+v, want stopped with a visible force-termination notice", snapshot)
	}
	runtime, err := manager.store.Runtime(ctx, "real-forced")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Status != session.StatusStopped || !strings.Contains(runtime.LastError, "force-terminated") || runtime.ProcessID != 0 {
		t.Fatalf("persisted runtime = %+v, want the forced stop recorded", runtime)
	}
}

func TestRealSessionGracefulExitIsNotReportedAsForced(t *testing.T) {
	realPwsh := lookRealPowerShell(t)
	manager, _, _ := newLifecycleTestManager(t)
	workingDir := t.TempDir()
	if _, err := session.SaveConfig(manager.configDirectory, session.Config{
		ID:         "real-graceful",
		Name:       "real-graceful",
		Tool:       session.CopilotCLI,
		WorkingDir: workingDir,
	}); err != nil {
		t.Fatal(err)
	}
	// Copilot's graceful exit is Ctrl+C, "/exit", Enter; this stand-in quits on that line.
	useRealPowerShell(t, manager, realPwsh,
		"[Console]::TreatControlCAsInput = $true; $typed = ''; while ($true) { $typed += [Console]::ReadKey($true).KeyChar; if ($typed.EndsWith('/exit')) { exit 0 } }")
	ctx := context.Background()
	if _, err := manager.Start(ctx, "real-graceful", false, 80, 24); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := manager.Stop(ctx, "real-graceful"); err != nil {
		t.Fatal(err)
	}
	snapshot := manager.sessions["real-graceful"].snapshot()
	if snapshot.Status != session.StatusStopped || snapshot.LastError != "" {
		t.Fatalf("snapshot = %+v, want a clean stopped state with no forced-termination notice", snapshot)
	}
}
