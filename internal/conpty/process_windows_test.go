package conpty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestProcessRunsPowerShellThroughConPTY(t *testing.T) {
	powershell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}

	command := "$utf8 = [System.Text.UTF8Encoding]::new($false); [Console]::InputEncoding = $utf8; [Console]::OutputEncoding = $utf8; $OutputEncoding = $utf8; chcp 65001 > $null; [Console]::Write('TAUSHELL_CONPTY_─_🙂'); [Console]::Write(([char]27) + '[31mTAUSHELL_COLOR' + ([char]27) + '[0m'); exit 7"
	process, err := Start(80, 24, powershell, t.TempDir(),
		"-NoLogo", "-NoProfile", "-Command",
		command)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	output := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := io.ReadAll(process.Output())
		output <- struct {
			data []byte
			err  error
		}{data: data, err: err}
	}()

	exited, exitCode, err := process.Wait(10 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !exited {
		_ = process.Terminate()
		t.Fatal("PowerShell did not exit within the test timeout")
	}
	process.CloseConsole()

	result := <-output
	if result.err != nil {
		t.Fatalf("read ConPTY output: %v", result.err)
	}
	if !strings.Contains(string(result.data), "TAUSHELL_CONPTY_─_🙂") {
		t.Fatalf("ConPTY output does not contain the marker: %q", result.data)
	}
	if !strings.Contains(string(result.data), "TAUSHELL_COLOR") || !strings.Contains(string(result.data), "\x1b") {
		t.Fatalf("ConPTY output did not preserve colored text: %q", result.data)
	}
	if exitCode != 7 {
		t.Fatalf("PowerShell exit code = %d, want 7", exitCode)
	}
}

func TestProcessCanResizeAndTerminate(t *testing.T) {
	powershell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}

	process, err := Start(80, 24, powershell, t.TempDir(),
		"-NoLogo", "-NoProfile", "-Command", "Start-Sleep -Seconds 30")
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	if process.PID() <= 0 {
		t.Fatalf("invalid child process ID %d", process.PID())
	}
	exited, _, err := process.Wait(100 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if exited {
		t.Fatal("PowerShell exited before resize and termination")
	}
	if err := process.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if err := process.Terminate(); err != nil {
		t.Fatal(err)
	}
	exited, _, err = process.Wait(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Fatal("PowerShell remained alive after job termination")
	}
}

func TestProcessForwardsUTF8Input(t *testing.T) {
	powershell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}
	oldInputCodePage, err := windows.GetConsoleCP()
	if err != nil {
		t.Fatal(err)
	}
	oldOutputCodePage, err := windows.GetConsoleOutputCP()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetConsoleCP(65001); err != nil {
		t.Fatal(err)
	}
	if err := windows.SetConsoleOutputCP(65001); err != nil {
		_ = windows.SetConsoleCP(oldInputCodePage)
		t.Fatal(err)
	}
	defer func() {
		_ = windows.SetConsoleOutputCP(oldOutputCodePage)
		_ = windows.SetConsoleCP(oldInputCodePage)
	}()

	readyFile := filepath.Join(t.TempDir(), "input-ready")
	command := fmt.Sprintf(
		"$utf8 = [System.Text.UTF8Encoding]::new($false); [Console]::InputEncoding = $utf8; [Console]::OutputEncoding = $utf8; chcp 65001 > $null; [System.IO.File]::WriteAllText('%s', 'ready'); $value = Read-Host; [Console]::Write('TAUSHELL_INPUT_' + $value); exit 0",
		strings.ReplaceAll(readyFile, "'", "''"),
	)
	process, err := Start(80, 24, powershell, t.TempDir(), "-NoLogo", "-NoProfile", "-Command", command)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	output := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, err := io.ReadAll(process.Output())
		output <- struct {
			data []byte
			err  error
		}{data: data, err: err}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(readyFile); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if _, err := os.Stat(readyFile); err != nil {
		_ = process.Terminate()
		t.Fatalf("PowerShell did not configure UTF-8 input: %v", err)
	}
	if err := process.Write([]byte("line-λ-🙂\r")); err != nil {
		t.Fatal(err)
	}
	exited, exitCode, err := process.Wait(10 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !exited {
		_ = process.Terminate()
		t.Fatal("PowerShell did not exit after receiving UTF-8 input")
	}
	process.CloseConsole()

	result := <-output
	if result.err != nil {
		t.Fatalf("read ConPTY output: %v", result.err)
	}
	if !strings.Contains(string(result.data), "TAUSHELL_INPUT_line-λ-🙂") {
		t.Fatalf("PowerShell did not receive UTF-8 input: %q", result.data)
	}
	if exitCode != 0 {
		t.Fatalf("PowerShell exit code = %d, want 0", exitCode)
	}
}

func TestTerminateStopsChildProcesses(t *testing.T) {
	powershell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		t.Skip("PowerShell 7 is unavailable")
	}
	commandPrompt, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip("Command Prompt is unavailable")
	}
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	command := fmt.Sprintf(
		"$child = Start-Process -FilePath '%s' -ArgumentList '/c ping -n 31 127.0.0.1 > nul' -PassThru; [System.IO.File]::WriteAllText('%s', [string]$child.Id); Start-Sleep -Seconds 30",
		strings.ReplaceAll(commandPrompt, "'", "''"),
		strings.ReplaceAll(pidFile, "'", "''"),
	)
	process, err := Start(80, 24, powershell, t.TempDir(), "-NoLogo", "-NoProfile", "-Command", command)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()

	var childPID int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			childPID, err = strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("parse child process ID %q: %v", data, err)
			}
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if childPID == 0 {
		t.Fatal("PowerShell did not start its child process within the test timeout")
	}
	alive, err := isProcessAlive(uint32(childPID))
	if err != nil {
		t.Fatal(err)
	}
	if !alive {
		t.Fatalf("child process %d exited before its parent was terminated", childPID)
	}

	if err := process.Terminate(); err != nil {
		t.Fatal(err)
	}
	exited, _, err := process.Wait(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !exited {
		t.Fatal("PowerShell remained alive after job termination")
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		alive, err = isProcessAlive(uint32(childPID))
		if err != nil {
			t.Fatal(err)
		}
		if !alive {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("child process %d remained alive after its job was terminated", childPID)
}

func isProcessAlive(processID uint32) (bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, processID)
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, nil
		}
		return false, fmt.Errorf("open process %d: %w", processID, err)
	}
	defer windows.CloseHandle(handle)
	result, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, fmt.Errorf("wait for process %d: %w", processID, err)
	}
	switch result {
	case uint32(windows.WAIT_TIMEOUT):
		return true, nil
	case windows.WAIT_OBJECT_0:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected wait result 0x%x for process %d", result, processID)
	}
}
