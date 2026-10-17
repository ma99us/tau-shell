//go:build windows

package sessionhost

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/session"
)

func TestSessionOutputWriterPreservesRawTerminalBytes(t *testing.T) {
	var got Event
	manager := &Manager{
		emit: func(event Event) {
			got = event
		},
	}
	input := []byte{0xf0, 0x9f, 0x8c, 0x8d, 0x1b, '[', '3', '1', 'm'}

	written, err := (sessionOutputWriter{manager: manager, id: "agent"}).Write(input)
	if err != nil {
		t.Fatal(err)
	}
	if written != len(input) {
		t.Fatalf("writer returned %d bytes, want %d", written, len(input))
	}
	decoded, err := base64.StdEncoding.DecodeString(got.DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, input) {
		t.Fatalf("event changed terminal bytes: got %v, want %v", decoded, input)
	}
}

func TestManagerKeepsConcurrentSessionIOIndependent(t *testing.T) {
	manager, factory, events := newLifecycleTestManager(t, "alpha", "beta")
	ctx := context.Background()
	for _, id := range []string{"alpha", "beta"} {
		if _, err := manager.Start(ctx, id, false, 80, 24); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
	}

	alpha := factory.process("alpha")
	beta := factory.process("beta")
	alphaOutput := []byte("alpha-🙂\x1b[31m")
	betaOutput := []byte("beta-🌍\x1b[32m")
	if _, err := alpha.outputWriter.Write(alphaOutput); err != nil {
		t.Fatal(err)
	}
	if _, err := beta.outputWriter.Write(betaOutput); err != nil {
		t.Fatal(err)
	}

	outputs := make(map[string][]byte)
	for len(outputs) < 2 {
		select {
		case event := <-events:
			if event.Kind != EventOutput {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(event.DataBase64)
			if err != nil {
				t.Fatal(err)
			}
			outputs[event.SessionID] = append(outputs[event.SessionID], data...)
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for both session outputs: got %v", outputs)
		}
	}
	if !bytes.Equal(outputs["alpha"], alphaOutput) || !bytes.Equal(outputs["beta"], betaOutput) {
		t.Fatalf("session outputs crossed or changed: alpha=%q beta=%q", outputs["alpha"], outputs["beta"])
	}

	if err := manager.Write("alpha", []byte("alpha-input")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Write("beta", []byte("beta-input")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Resize("alpha", 100, 32); err != nil {
		t.Fatal(err)
	}
	if err := manager.Resize("beta", 120, 40); err != nil {
		t.Fatal(err)
	}
	if got := alpha.input(); got != "alpha-input" {
		t.Errorf("alpha input = %q, want alpha-input", got)
	}
	if got := beta.input(); got != "beta-input" {
		t.Errorf("beta input = %q, want beta-input", got)
	}
	if columns, rows := alpha.dimensions(); columns != 100 || rows != 32 {
		t.Errorf("alpha dimensions = %dx%d, want 100x32", columns, rows)
	}
	if columns, rows := beta.dimensions(); columns != 120 || rows != 40 {
		t.Errorf("beta dimensions = %dx%d, want 120x40", columns, rows)
	}

	if err := manager.StopAll(ctx); err != nil {
		t.Fatal(err)
	}
	for id, process := range map[string]*fakeSessionProcess{"alpha": alpha, "beta": beta} {
		if snapshot := manager.sessions[id].snapshot(); snapshot.Status != session.StatusStopped {
			t.Errorf("%s status = %q, want stopped (detail: %s)", id, snapshot.Status, snapshot.LastError)
		}
		if !process.isClosed() {
			t.Errorf("%s process was not closed", id)
		}
	}
}

func TestManagerReportsForcedTermination(t *testing.T) {
	manager, factory, _ := newLifecycleTestManager(t, "forced")
	if _, err := manager.Start(context.Background(), "forced", false, 80, 24); err != nil {
		t.Fatal(err)
	}
	process := factory.process("forced")
	process.setInterruptError(errors.New("interrupt unavailable"))

	if err := manager.Stop(context.Background(), "forced"); err != nil {
		t.Fatal(err)
	}
	snapshot := manager.sessions["forced"].snapshot()
	if snapshot.Status != session.StatusStopped {
		t.Fatalf("status = %q, want stopped", snapshot.Status)
	}
	if !strings.Contains(snapshot.LastError, "force-terminated") {
		t.Fatalf("forced termination was not reported: %q", snapshot.LastError)
	}
	if !process.isClosed() {
		t.Fatal("force-terminated process was not closed")
	}
}

func TestManagerCloseWaitsForSupervisorsAndIsIdempotent(t *testing.T) {
	manager, factory, _ := newLifecycleTestManager(t, "alpha", "beta")
	supervisorsBefore := countManagerSupervisorGoroutines()
	for _, id := range []string{"alpha", "beta"} {
		if _, err := manager.Start(context.Background(), id, false, 80, 24); err != nil {
			t.Fatalf("start %s: %v", id, err)
		}
	}

	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	for _, id := range []string{"alpha", "beta"} {
		current := manager.sessions[id]
		select {
		case <-current.done:
		default:
			t.Errorf("%s supervisor was still running after Close returned", id)
		}
		if !factory.process(id).isClosed() {
			t.Errorf("%s process resources were not closed", id)
		}
		process := factory.process(id)
		if _, err := process.outputReader.Read(make([]byte, 1)); !errors.Is(err, os.ErrClosed) {
			t.Errorf("%s output reader is not closed: %v", id, err)
		}
		if _, err := process.outputWriter.Write([]byte{0}); !errors.Is(err, os.ErrClosed) {
			t.Errorf("%s output writer is not closed: %v", id, err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for countManagerSupervisorGoroutines() != supervisorsBefore && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := countManagerSupervisorGoroutines(); got != supervisorsBefore {
		t.Errorf("manager supervisor goroutines = %d after Close, want baseline %d", got, supervisorsBefore)
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := manager.Start(context.Background(), "alpha", false, 80, 24); err == nil {
		t.Fatal("Start succeeded after manager.Close")
	}
}

func TestManagerCloseCanRetryStoreCloseFailure(t *testing.T) {
	manager, _, _ := newLifecycleTestManager(t, "retry-close")
	closeErr := errors.New("database close failed")
	originalClose := manager.closeStore
	calls := 0
	manager.closeStore = func() error {
		calls++
		if calls == 1 {
			return closeErr
		}
		return originalClose()
	}

	if err := manager.Close(context.Background()); !errors.Is(err, closeErr) {
		t.Fatalf("first Close error = %v, want %v", err, closeErr)
	}
	if manager.storeClosed {
		t.Fatal("store was marked closed after its close operation failed")
	}
	if _, err := manager.Start(context.Background(), "retry-close", false, 80, 24); err == nil {
		t.Fatal("Start succeeded after manager shutdown began")
	}
	if err := manager.Close(context.Background()); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if !manager.storeClosed || calls != 2 {
		t.Fatalf("storeClosed=%v, close calls=%d; want true and two calls", manager.storeClosed, calls)
	}
}

func TestManagerStopAllCanRetryAfterContextTimeout(t *testing.T) {
	manager, factory, _ := newLifecycleTestManager(t, "retry")
	if _, err := manager.Start(context.Background(), "retry", false, 80, 24); err != nil {
		t.Fatal(err)
	}
	process := factory.process("retry")
	process.setIgnoreInterrupt(true)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.StopAll(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopAll error = %v, want context deadline exceeded", err)
	}
	if !manager.sessions["retry"].isRunning() {
		t.Fatal("timed-out graceful stop unexpectedly changed the running state")
	}
	if process.isClosed() {
		t.Fatal("timed-out graceful stop closed a still-running process")
	}

	process.setIgnoreInterrupt(false)
	if err := manager.StopAll(context.Background()); err != nil {
		t.Fatalf("retry StopAll: %v", err)
	}
	select {
	case <-manager.sessions["retry"].done:
	default:
		t.Fatal("supervisor was still running after retry StopAll")
	}
	if !process.isClosed() {
		t.Fatal("process was not closed after retry StopAll")
	}
}

func TestManagerRecordsHarnessProcessExit(t *testing.T) {
	manager, factory, _ := newLifecycleTestManager(t, "exited")
	if _, err := manager.Start(context.Background(), "exited", false, 80, 24); err != nil {
		t.Fatal(err)
	}
	process := factory.process("exited")
	process.exit(7)

	select {
	case <-manager.sessions["exited"].done:
	case <-time.After(2 * time.Second):
		t.Fatal("session supervisor did not finish after process exit")
	}
	snapshot := manager.sessions["exited"].snapshot()
	if snapshot.Status != session.StatusFailed || !strings.Contains(snapshot.LastError, "code 7") {
		t.Fatalf("exit snapshot = %+v, want failed status and exit code", snapshot)
	}
	if !process.isClosed() {
		t.Fatal("exited process was not closed")
	}
}

func TestManagerReconcilesRunningStateWithoutProcessID(t *testing.T) {
	manager, _, _ := newLifecycleTestManager(t, "stale")
	ctx := context.Background()
	if _, err := manager.store.BeginFresh(ctx, "stale", session.ClaudeCode); err != nil {
		t.Fatal(err)
	}
	if err := manager.store.MarkRunning(ctx, "stale", 0); err != nil {
		t.Fatal(err)
	}

	snapshots, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("got %d snapshots, want one", len(snapshots))
	}
	if snapshots[0].Status != session.StatusFailed || snapshots[0].LastError == "" {
		t.Fatalf("stale runtime snapshot = %+v, want observable failed state", snapshots[0])
	}
	if !snapshots[0].HasResumeID {
		t.Fatal("stale-runtime recovery discarded the harness resume ID")
	}
}

func TestManagerPreservesLiveProcessFromAnotherWindow(t *testing.T) {
	manager, _, _ := newLifecycleTestManager(t, "other-window")
	ctx := context.Background()
	runtime, err := manager.store.BeginFresh(ctx, "other-window", session.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.store.MarkRunning(ctx, "other-window", os.Getpid()); err != nil {
		t.Fatal(err)
	}

	snapshots, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("got %d snapshots, want one", len(snapshots))
	}
	snapshot := snapshots[0]
	if snapshot.Status != session.StatusRunning || snapshot.RunningHere {
		t.Fatalf("other-window snapshot = %+v, want running but not managed here", snapshot)
	}
	if !snapshot.HasResumeID {
		t.Fatalf("live runtime lost its harness session ID %q", runtime.HarnessSessionID)
	}
}

func TestManagerReconcilesAbandonedStartingState(t *testing.T) {
	root := t.TempDir()
	configDirectory := filepath.Join(root, "sessions")
	workingDirectory := filepath.Join(root, "work")
	for _, directory := range []string{configDirectory, workingDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := session.SaveConfig(configDirectory, session.Config{
		ID:         "abandoned",
		Name:       "Abandoned",
		Tool:       session.ClaudeCode,
		WorkingDir: workingDirectory,
	}); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "state.db")
	manager, err := New(configDirectory, databasePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(context.Background()); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	ctx := context.Background()
	original, err := manager.store.BeginFresh(ctx, "abandoned", session.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, updateErr := database.ExecContext(ctx, `UPDATE session_runtime SET updated_at = ? WHERE session_id = ?`,
		time.Now().Add(-startupRecoveryDelay-time.Second).UTC().Format(time.RFC3339Nano), "abandoned")
	closeErr := database.Close()
	if updateErr != nil || closeErr != nil {
		t.Fatalf("age abandoned starting state: %v", errors.Join(updateErr, closeErr))
	}

	snapshots, err := manager.List(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Status != session.StatusFailed {
		t.Fatalf("abandoned startup snapshots = %+v, want one failed session", snapshots)
	}
	if !strings.Contains(snapshots[0].LastError, "stopped while starting") {
		t.Fatalf("abandoned startup error = %q", snapshots[0].LastError)
	}
	if !snapshots[0].HasResumeID || snapshots[0].RunningHere {
		t.Fatalf("abandoned startup lost resume state or appears live: %+v (original ID %q)", snapshots[0], original.HarnessSessionID)
	}
}

func TestManagerRestoreFailurePreservesResumeID(t *testing.T) {
	manager, factory, _ := newLifecycleTestManager(t, "restore")
	ctx := context.Background()
	original, err := manager.store.BeginFresh(ctx, "restore", session.ClaudeCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.store.Finish(ctx, "restore", session.StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	factory.setStartError(errors.New("fake ConPTY launch failure"))

	_, err = manager.Start(ctx, "restore", true, 80, 24)
	if err == nil || !strings.Contains(err.Error(), "fake ConPTY launch failure") {
		t.Fatalf("restore error = %v, want fake ConPTY launch failure", err)
	}
	runtime, err := manager.store.Runtime(ctx, "restore")
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Status != session.StatusFailed {
		t.Errorf("restore status = %q, want failed", runtime.Status)
	}
	if runtime.HarnessSessionID != original.HarnessSessionID {
		t.Errorf("failed restore changed resume ID: got %q, want %q", runtime.HarnessSessionID, original.HarnessSessionID)
	}
}

func countManagerSupervisorGoroutines() int {
	var dump bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&dump, 2); err != nil {
		panic(err)
	}
	return strings.Count(dump.String(), "github.com/ma99us/tau-shell/internal/sessionhost.(*Manager).supervise")
}

func newLifecycleTestManager(t *testing.T, ids ...string) (*Manager, *fakeProcessFactory, <-chan Event) {
	t.Helper()
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	configDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, executable := range []string{"claude.exe", "copilot.exe", "pwsh.exe"} {
		if err := os.WriteFile(filepath.Join(binDir, executable), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, id := range ids {
		workingDir := filepath.Join(root, "work", id)
		if err := os.MkdirAll(workingDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := session.SaveConfig(configDir, session.Config{
			ID:         id,
			Name:       id,
			Tool:       session.ClaudeCode,
			WorkingDir: workingDir,
		}); err != nil {
			t.Fatal(err)
		}
	}
	eventChannel := make(chan Event, 32)
	manager, err := New(configDir, filepath.Join(root, "state.db"), func(event Event) {
		eventChannel <- event
	})
	if err != nil {
		t.Fatal(err)
	}
	factory := &fakeProcessFactory{processes: make(map[string]*fakeSessionProcess)}
	manager.startProcess = factory.start
	t.Cleanup(func() {
		if err := manager.Close(context.Background()); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	return manager, factory, eventChannel
}

type fakeProcessFactory struct {
	mu        sync.Mutex
	processes map[string]*fakeSessionProcess
	startErr  error
}

func (factory *fakeProcessFactory) start(_, _ int, _, directory string, _ ...string) (sessionProcess, error) {
	factory.mu.Lock()
	defer factory.mu.Unlock()
	if factory.startErr != nil {
		err := factory.startErr
		factory.startErr = nil
		return nil, err
	}
	process, err := newFakeSessionProcess()
	if err != nil {
		return nil, err
	}
	factory.processes[filepath.Base(directory)] = process
	return process, nil
}

func (factory *fakeProcessFactory) process(id string) *fakeSessionProcess {
	factory.mu.Lock()
	defer factory.mu.Unlock()
	process := factory.processes[id]
	if process == nil {
		panic(fmt.Sprintf("no fake process for session %q", id))
	}
	return process
}

func (factory *fakeProcessFactory) setStartError(err error) {
	factory.mu.Lock()
	factory.startErr = err
	factory.mu.Unlock()
}

type fakeSessionProcess struct {
	outputReader    *os.File
	outputWriter    *os.File
	done            chan struct{}
	exitOnce        sync.Once
	closeOnce       sync.Once
	mu              sync.Mutex
	exitCode        uint32
	inputData       []byte
	columns         int
	rows            int
	interruptErr    error
	ignoreInterrupt bool
	closed          bool
}

func newFakeSessionProcess() (*fakeSessionProcess, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &fakeSessionProcess{
		outputReader: reader,
		outputWriter: writer,
		done:         make(chan struct{}),
	}, nil
}

func (process *fakeSessionProcess) PID() int {
	return os.Getpid()
}

func (process *fakeSessionProcess) Output() *os.File {
	return process.outputReader
}

func (process *fakeSessionProcess) Write(data []byte) error {
	process.mu.Lock()
	process.inputData = append(process.inputData, data...)
	process.mu.Unlock()
	return nil
}

func (process *fakeSessionProcess) Resize(columns, rows int) error {
	process.mu.Lock()
	process.columns, process.rows = columns, rows
	process.mu.Unlock()
	return nil
}

func (process *fakeSessionProcess) Interrupt() error {
	process.mu.Lock()
	err := process.interruptErr
	ignore := process.ignoreInterrupt
	process.mu.Unlock()
	if err != nil {
		return err
	}
	if !ignore {
		process.exit(0)
	}
	return nil
}

func (process *fakeSessionProcess) Terminate() error {
	process.exit(1)
	return nil
}

func (process *fakeSessionProcess) CloseConsole() {
	process.closeWriter()
}

func (process *fakeSessionProcess) Wait(timeout time.Duration) (bool, uint32, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-process.done:
		process.mu.Lock()
		code := process.exitCode
		process.mu.Unlock()
		return true, code, nil
	case <-timer.C:
		return false, 0, nil
	}
}

func (process *fakeSessionProcess) Close() error {
	process.closeWriter()
	_ = process.outputReader.Close()
	process.mu.Lock()
	process.closed = true
	process.mu.Unlock()
	return nil
}

func (process *fakeSessionProcess) exit(code uint32) {
	process.exitOnce.Do(func() {
		process.mu.Lock()
		process.exitCode = code
		process.mu.Unlock()
		close(process.done)
		process.closeWriter()
	})
}

func (process *fakeSessionProcess) closeWriter() {
	process.closeOnce.Do(func() {
		_ = process.outputWriter.Close()
	})
}

func (process *fakeSessionProcess) setInterruptError(err error) {
	process.mu.Lock()
	process.interruptErr = err
	process.mu.Unlock()
}

func (process *fakeSessionProcess) setIgnoreInterrupt(ignore bool) {
	process.mu.Lock()
	process.ignoreInterrupt = ignore
	process.mu.Unlock()
}

func (process *fakeSessionProcess) input() string {
	process.mu.Lock()
	defer process.mu.Unlock()
	return string(process.inputData)
}

func (process *fakeSessionProcess) dimensions() (int, int) {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.columns, process.rows
}

func (process *fakeSessionProcess) isClosed() bool {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.closed
}
