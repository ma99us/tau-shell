package conpty

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBundledLibraryRequiresBothFiles(t *testing.T) {
	directory := t.TempDir()
	if got := BundledLibrary(directory); got != "" {
		t.Fatalf("empty directory resolved %q", got)
	}
	if err := os.WriteFile(filepath.Join(directory, "conpty.dll"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := BundledLibrary(directory); got != "" {
		t.Fatalf("missing OpenConsole.exe resolved %q", got)
	}
	if err := os.WriteFile(filepath.Join(directory, "OpenConsole.exe"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(directory, "conpty.dll")
	if got := BundledLibrary(directory); got != want {
		t.Fatalf("BundledLibrary = %q, want %q", got, want)
	}
	if got := BundledLibrary(""); got != "" {
		t.Fatalf("empty path resolved %q", got)
	}
}

func TestStartWithLibraryRejectsRelativePath(t *testing.T) {
	if _, err := StartWithLibrary("conpty.dll", 80, 25, "cmd.exe", t.TempDir()); err == nil {
		t.Fatal("relative library path was accepted")
	}
}

func TestStartPreferredFallsBackToInbox(t *testing.T) {
	cmd := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")

	process, fallbackErr, err := StartPreferred(t.TempDir(), 80, 25, cmd, t.TempDir(), "/c", "exit", "0")
	if err != nil {
		t.Fatalf("start without bundle: %v", err)
	}
	if fallbackErr != nil || process.Backend() != BackendInbox {
		t.Fatalf("backend=%s fallbackErr=%v, want inbox without fallback error", process.Backend(), fallbackErr)
	}
	waitAndClose(t, process)

	invalid := t.TempDir()
	for _, name := range []string{"conpty.dll", "OpenConsole.exe"} {
		if err := os.WriteFile(filepath.Join(invalid, name), []byte("not a binary"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	process, fallbackErr, err = StartPreferred(invalid, 80, 25, cmd, t.TempDir(), "/c", "exit", "0")
	if err != nil {
		t.Fatalf("start with invalid bundle: %v", err)
	}
	if fallbackErr == nil || process.Backend() != BackendInbox {
		t.Fatalf("backend=%s fallbackErr=%v, want inbox with reported fallback error", process.Backend(), fallbackErr)
	}
	waitAndClose(t, process)
}

func waitAndClose(t *testing.T, process *Process) {
	t.Helper()
	go func() {
		buffer := make([]byte, 1024)
		for {
			if _, err := process.Output().Read(buffer); err != nil {
				return
			}
		}
	}()
	if exited, _, err := process.Wait(5 * time.Second); err != nil || !exited {
		_ = process.Terminate()
		t.Errorf("process did not exit: exited=%v err=%v", exited, err)
	}
	if err := process.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
