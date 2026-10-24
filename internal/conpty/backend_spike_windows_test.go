package conpty

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestConPTYBackendSpike compares inbox ConPTY with a bundled conpty.dll for
// mouse-mode passthrough, query delivery, and per-cycle handle residue. It is
// evidence gathering for the bundled-ConPTY spike and only runs when
// TAUSHELL_CONPTY_SPIKE_DLL names an absolute conpty.dll path.
func TestConPTYBackendSpike(t *testing.T) {
	library := os.Getenv("TAUSHELL_CONPTY_SPIKE_DLL")
	if library == "" {
		t.Skip("set TAUSHELL_CONPTY_SPIKE_DLL to run the bundled ConPTY spike")
	}
	fixture := buildTerminalFixture(t)
	backends := []struct {
		name  string
		start func(width, height int, executable, directory string, args ...string) (*Process, error)
	}{
		{"inbox", Start},
		{"bundled", func(width, height int, executable, directory string, args ...string) (*Process, error) {
			return StartWithLibrary(library, width, height, executable, directory, args...)
		}},
	}
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			runFixtureSpike(t, backend.start, fixture)
			measureHandleResidue(t, backend.start, fixture)
		})
	}
}

func buildTerminalFixture(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "terminal-fixture.exe")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-o", output, "github.com/ma99us/tau-shell/cmd/terminal-fixture")
	if combined, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build terminal fixture: %v\n%s", err, combined)
	}
	return output
}

func runFixtureSpike(t *testing.T, start func(int, int, string, string, ...string) (*Process, error), fixture string) {
	eventLog := filepath.Join(t.TempDir(), "events.log")
	t.Setenv("TAUSHELL_FIXTURE_LOG", eventLog)
	process, err := start(100, 30, fixture, filepath.Dir(fixture))
	if err != nil {
		t.Fatalf("start fixture: %v", err)
	}
	defer process.Close()

	var mu sync.Mutex
	var stream bytes.Buffer
	replies := map[string]int{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 4096)
		for {
			n, err := process.Output().Read(buffer)
			if n > 0 {
				mu.Lock()
				stream.Write(buffer[:n])
				// Answer queries the way xterm.js would.
				for query, reply := range map[string]string{"\x1b[5n": "\x1b[0n", "\x1b[6n": "\x1b[3;7R"} {
					for seen := strings.Count(stream.String(), query); replies[query] < seen; replies[query]++ {
						_ = process.Write([]byte(reply))
					}
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	time.Sleep(1500 * time.Millisecond)
	steps := []string{"\x1b[<0;5;5M\x1b[<0;5;5m", "d", "c", "m", "\x1b[<0;6;6M\x1b[<0;6;6m", "q"}
	for _, step := range steps {
		if err := process.Write([]byte(step)); err != nil {
			t.Fatalf("write %q: %v", step, err)
		}
		time.Sleep(700 * time.Millisecond)
	}
	exited, code, err := process.Wait(5 * time.Second)
	if err != nil || !exited {
		_ = process.Terminate()
		t.Fatalf("fixture did not exit cleanly: exited=%v code=%d err=%v", exited, code, err)
	}
	process.CloseConsole()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}

	mu.Lock()
	output := stream.String()
	queryReplies := map[string]int{"DSR": replies["\x1b[5n"], "CPR": replies["\x1b[6n"]}
	mu.Unlock()
	events, err := os.ReadFile(eventLog)
	if err != nil {
		t.Fatalf("read fixture events: %v", err)
	}
	t.Logf("output: mouse ?1000h=%d ?1006h=%d DSR-query=%d CPR-query=%d; host replies sent=%v",
		strings.Count(output, "\x1b[?1000h"), strings.Count(output, "\x1b[?1006h"),
		strings.Count(output, "\x1b[5n"), strings.Count(output, "\x1b[6n"), queryReplies)
	var observed []string
	for _, line := range strings.Split(strings.TrimSpace(string(events)), "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{"mouse", "device-status-report", "cursor-position-report", "terminal-control-input", "exit"} {
			if strings.HasPrefix(line, prefix) {
				observed = append(observed, line)
				break
			}
		}
	}
	t.Logf("fixture events: %s", strings.Join(observed, " | "))
}

func measureHandleResidue(t *testing.T, start func(int, int, string, string, ...string) (*Process, error), fixture string) {
	const cycles = 10
	runCycle := func() {
		process, err := start(80, 25, fixture, filepath.Dir(fixture), "--version")
		if err != nil {
			t.Fatalf("start fixture: %v", err)
		}
		go func() { _, _ = bytes.NewBuffer(nil).ReadFrom(process.Output()) }()
		if exited, _, err := process.Wait(5 * time.Second); err != nil || !exited {
			_ = process.Terminate()
		}
		if err := process.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	runCycle()
	time.Sleep(500 * time.Millisecond)
	before := processHandleCount(t)
	for i := 0; i < cycles; i++ {
		runCycle()
	}
	time.Sleep(time.Second)
	after := processHandleCount(t)
	t.Logf("handle count after %d cycles: before=%d after=%d delta=%d", cycles, before, after, int(after)-int(before))
}

func processHandleCount(t *testing.T) uint32 {
	t.Helper()
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")
	var count uint32
	result, _, err := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&count)))
	if result == 0 {
		t.Fatalf("GetProcessHandleCount: %v", err)
	}
	return count
}
