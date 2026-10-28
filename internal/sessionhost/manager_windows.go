package sessionhost

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/conpty"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/usage"
)

const (
	startupRecoveryDelay = time.Minute
	interruptWait        = 3 * time.Second
	terminateWait        = 5 * time.Second
)

// gracefulExitWait is how long a harness gets to quit after its exit input.
var gracefulExitWait = 8 * time.Second

type EventKind string

const (
	EventOutput EventKind = "output"
	EventState  EventKind = "state"
)

type Event struct {
	Kind       EventKind `json:"kind"`
	SessionID  string    `json:"sessionId"`
	DataBase64 string    `json:"dataBase64,omitempty"`
	Session    Snapshot  `json:"session,omitempty"`
}

type Snapshot struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Color       string                `json:"color,omitempty"`
	AutoResume  bool                  `json:"autoResume"`
	Tool        session.Harness       `json:"tool"`
	WorkingDir  string                `json:"workingDir"`
	Status      session.RuntimeStatus `json:"status"`
	LastError   string                `json:"lastError,omitempty"`
	RunningHere bool                  `json:"runningHere"`
	HasResumeID bool                  `json:"hasResumeId"`
	ResumeNote  string                `json:"resumeNote,omitempty"`
}

type managedSession struct {
	config      session.Config
	process     sessionProcess
	done        chan struct{}
	mu          sync.RWMutex
	stopMu      sync.Mutex
	status      session.RuntimeStatus
	detail      string
	forced      bool
	hasResumeID bool
}

type sessionProcess interface {
	PID() int
	Output() *os.File
	Write([]byte) error
	Resize(int, int) error
	Interrupt() error
	Terminate() error
	CloseConsole()
	Wait(time.Duration) (bool, uint32, error)
	Close() error
}

type processFactory func(int, int, string, string, ...string) (sessionProcess, error)

type Manager struct {
	configDirectory string
	store           *session.Store
	closeStore      func() error
	emit            func(Event)
	startProcess    processFactory
	mu              sync.Mutex
	shutdownMu      sync.Mutex
	sessions        map[string]*managedSession
	closed          bool
	storeClosed     bool
	ledger          *usage.Ledger
	prices          *usage.PriceStore
	usageMu         sync.Mutex
	lastIngest      map[string]time.Time
	cancelPrices    context.CancelFunc
}

func New(configDirectory, databasePath string, emit func(Event)) (*Manager, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create TauShell data directory: %w", err)
	}
	store, err := session.OpenStore(databasePath)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		configDirectory: configDirectory,
		store:           store,
		closeStore:      store.Close,
		emit:            emit,
		startProcess: func(columns, rows int, executable, directory string, args ...string) (sessionProcess, error) {
			process, fallbackErr, err := conpty.StartPreferred(conpty.ExecutableDirectory(), columns, rows, executable, directory, args...)
			if fallbackErr != nil {
				diagnostics.Warn("bundled ConPTY unavailable; using inbox ConPTY", fallbackErr)
			}
			if err != nil {
				return nil, err
			}
			diagnostics.Info("ConPTY backend selected", "backend", process.Backend())
			return process, nil
		},
		sessions: make(map[string]*managedSession),
	}
	manager.openUsage(databasePath)
	return manager, nil
}

// HarnessModel reports the model the harness last recorded for the session,
// or "" when unknown or the session has no harness conversation yet.
func (manager *Manager) HarnessModel(ctx context.Context, id string) (string, error) {
	config, err := session.FindConfig(manager.configDirectory, id)
	if err != nil {
		return "", err
	}
	runtime, err := manager.store.Runtime(ctx, id)
	if err != nil {
		return "", err
	}
	return harness.CurrentModel(config.Tool, runtime.HarnessSessionID), nil
}

func (manager *Manager) List(ctx context.Context) ([]Snapshot, error) {
	configs, err := session.ListConfigs(manager.configDirectory)
	if err != nil {
		return nil, err
	}
	result := make([]Snapshot, 0, len(configs))
	for _, config := range configs {
		runtime, err := manager.recoverRuntime(ctx, config.ID)
		if err != nil {
			return nil, err
		}
		snapshot := Snapshot{
			ID:          config.ID,
			Name:        config.Name,
			Color:       config.Color,
			AutoResume:  config.AutoResume,
			Tool:        config.Tool,
			WorkingDir:  config.WorkingDir,
			Status:      runtime.Status,
			LastError:   runtime.LastError,
			HasResumeID: runtime.HarnessSessionID != "",
		}
		if snapshot.HasResumeID {
			if adapter, adapterErr := harness.For(config.Tool); adapterErr == nil {
				if resumeErr := adapter.CheckResume(runtime.HarnessSessionID); resumeErr != nil {
					snapshot.ResumeNote = resumeErr.Error()
				}
			}
		}
		result = append(result, snapshot)
	}
	manager.mu.Lock()
	for index := range result {
		if current := manager.sessions[result[index].ID]; current != nil {
			result[index] = current.snapshot()
		}
	}
	manager.mu.Unlock()
	if err := manager.applyOrder(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

// applyOrder sorts snapshots by the saved display order; sessions without a
// saved position keep their relative order after the ordered ones.
func (manager *Manager) applyOrder(ctx context.Context, snapshots []Snapshot) error {
	order, err := manager.store.SessionOrder(ctx)
	if err != nil {
		return err
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		left, leftOK := order[snapshots[i].ID]
		right, rightOK := order[snapshots[j].ID]
		if leftOK != rightOK {
			return leftOK
		}
		return leftOK && left < right
	})
	return nil
}

// SetOrder saves the display order. Every id must be an existing session;
// sessions not listed keep their current relative order after the listed ones.
func (manager *Manager) SetOrder(ctx context.Context, ids []string) error {
	snapshots, err := manager.List(ctx)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(snapshots))
	for _, snapshot := range snapshots {
		known[snapshot.ID] = true
	}
	final := make([]string, 0, len(snapshots))
	seen := map[string]bool{}
	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("unknown session %q", id)
		}
		if !seen[id] {
			seen[id] = true
			final = append(final, id)
		}
	}
	for _, snapshot := range snapshots {
		if !seen[snapshot.ID] {
			final = append(final, snapshot.ID)
		}
	}
	return manager.store.SetSessionOrder(ctx, final)
}

// PlaceAfter saves the display order with id directly after afterID. Both
// sessions must already exist.
func (manager *Manager) PlaceAfter(ctx context.Context, id, afterID string) error {
	snapshots, err := manager.List(ctx)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(snapshots))
	found := false
	for _, snapshot := range snapshots {
		if snapshot.ID == id {
			continue
		}
		ids = append(ids, snapshot.ID)
		if snapshot.ID == afterID {
			ids = append(ids, id)
			found = true
		}
	}
	if !found {
		return fmt.Errorf("unknown session %q", afterID)
	}
	return manager.store.SetSessionOrder(ctx, ids)
}

const autoResumeAllSetting = "auto_resume_all"

// AutoResumeAll reports the global "resume every session on startup" preference.
func (manager *Manager) AutoResumeAll(ctx context.Context) (bool, error) {
	value, err := manager.store.Setting(ctx, autoResumeAllSetting)
	return value == "true", err
}

// SetAutoResumeAll saves the global startup auto-resume preference.
func (manager *Manager) SetAutoResumeAll(ctx context.Context, enabled bool) error {
	return manager.store.SetSetting(ctx, autoResumeAllSetting, strconv.FormatBool(enabled))
}

// UpdateDisplay persists a new name, color or auto_resume value for a session
// and applies it to the running session, if any, without touching its process.
func (manager *Manager) UpdateDisplay(id, field, value string) error {
	config, err := session.UpdateConfigField(manager.configDirectory, id, field, value)
	if err != nil {
		return err
	}
	if current := manager.get(id); current != nil {
		current.mu.Lock()
		current.config.Name = config.Name
		current.config.Color = config.Color
		current.config.AutoResume = config.AutoResume
		current.mu.Unlock()
	}
	return nil
}

func (manager *Manager) Start(ctx context.Context, id string, restoring bool, columns, rows int) (Snapshot, error) {
	return manager.start(ctx, id, restoring, false, columns, rows)
}

func (manager *Manager) StartFresh(ctx context.Context, id string, columns, rows int) (Snapshot, error) {
	// Capture the old conversation's unread tail before its ID is replaced.
	manager.ingestSession(ctx, id, true)
	return manager.start(ctx, id, false, true, columns, rows)
}

func (manager *Manager) start(ctx context.Context, id string, restoring, fresh bool, columns, rows int) (Snapshot, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return Snapshot{}, errors.New("session manager is shutting down")
	}
	if current := manager.sessions[id]; current != nil {
		if current.isRunning() {
			return current.snapshot(), nil
		}
	}

	config, err := session.FindConfig(manager.configDirectory, id)
	if err != nil {
		return Snapshot{}, err
	}
	adapter, err := harness.For(config.Tool)
	if err != nil {
		return Snapshot{}, err
	}
	harnessExecutable, err := adapter.FindExecutable()
	if err != nil {
		return Snapshot{}, err
	}
	powershell, err := exec.LookPath("pwsh.exe")
	if err != nil {
		return Snapshot{}, fmt.Errorf("PowerShell 7 (pwsh.exe) is required and must be on PATH: %w", err)
	}
	runtime, err := manager.recoverRuntime(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	if runtime.Status == session.StatusRunning || runtime.Status == session.StatusStarting {
		return Snapshot{}, fmt.Errorf("session %q is active in another TauShell process", id)
	}
	if restoring && runtime.HarnessSessionID != "" {
		if resumeErr := adapter.CheckResume(runtime.HarnessSessionID); resumeErr != nil {
			return Snapshot{}, resumeErr
		}
	}
	if fresh {
		runtime, err = manager.store.BeginFresh(ctx, id, config.Tool)
	} else {
		runtime, err = manager.store.Begin(ctx, id, config.Tool, restoring)
	}
	if err != nil {
		return Snapshot{}, err
	}
	args := append([]string(nil), config.LaunchArgs...)
	if restoring {
		args = append(args, adapter.ResumeArgs(runtime.HarnessSessionID)...)
	} else {
		args = append(args, adapter.StartArgs(runtime.HarnessSessionID)...)
	}
	command := harness.PowerShellInvocation(harnessExecutable, args, config.Environment, config.UTF8Enabled())
	if columns < 2 || rows < 2 {
		columns, rows = 80, 24
	}
	process, err := manager.startProcess(columns, rows, powershell, config.WorkingDir,
		"-NoLogo", "-NoProfile", "-Command", command)
	if err != nil {
		finishErr := manager.store.Finish(ctx, id, session.StatusFailed, err.Error())
		return Snapshot{}, errors.Join(err, finishErr)
	}
	if err := manager.store.MarkRunning(ctx, id, process.PID()); err != nil {
		_ = process.Terminate()
		closeErr := process.Close()
		finishErr := manager.store.Finish(ctx, id, session.StatusFailed, err.Error())
		return Snapshot{}, errors.Join(fmt.Errorf("persist launched session: %w", err), closeErr, finishErr)
	}
	current := &managedSession{
		config:      config,
		process:     process,
		done:        make(chan struct{}),
		status:      session.StatusRunning,
		hasResumeID: runtime.HarnessSessionID != "",
	}
	manager.sessions[id] = current
	manager.publish(Event{Kind: EventState, SessionID: id, Session: current.snapshot()})
	diagnostics.Info("session process started",
		"session_id", id,
		"tool", config.Tool,
		"pid", process.PID(),
		"restoring", restoring,
		"fresh", fresh,
		"columns", columns,
		"rows", rows,
	)
	go manager.supervise(current)
	return current.snapshot(), nil
}

func (manager *Manager) Write(id string, input []byte) error {
	if len(input) > 64*1024 {
		return errors.New("terminal input exceeds the 64 KiB limit")
	}
	current := manager.get(id)
	if current == nil || !current.isRunning() {
		return fmt.Errorf("session %q is not running in this TauShell window", id)
	}
	if err := current.process.Write(input); err != nil {
		return fmt.Errorf("write terminal input for %q: %w", id, err)
	}
	return nil
}

func (manager *Manager) Resize(id string, columns, rows int) error {
	if columns < 2 || rows < 2 || columns > 32767 || rows > 32767 {
		return fmt.Errorf("invalid terminal dimensions %dx%d", columns, rows)
	}
	current := manager.get(id)
	if current == nil || !current.isRunning() {
		return fmt.Errorf("session %q is not running in this TauShell window", id)
	}
	if err := current.process.Resize(columns, rows); err != nil {
		return fmt.Errorf("resize session %q: %w", id, err)
	}
	return nil
}

// Delete removes a session's YAML and runtime state. It refuses active sessions
// and never touches the harness's own conversation data or the working directory.
func (manager *Manager) Delete(ctx context.Context, id string) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return errors.New("session manager is shutting down")
	}
	if current := manager.sessions[id]; current != nil && current.isRunning() {
		return fmt.Errorf("session %q is running; stop it before deleting", id)
	}
	if _, err := session.FindConfig(manager.configDirectory, id); err != nil {
		return err
	}
	runtime, err := manager.recoverRuntime(ctx, id)
	if err != nil {
		return err
	}
	if runtime.Status == session.StatusRunning || runtime.Status == session.StatusStarting {
		return fmt.Errorf("session %q is active in another TauShell process", id)
	}
	manager.ingestSession(ctx, id, true)
	if err := session.DeleteConfig(manager.configDirectory, id); err != nil {
		return err
	}
	delete(manager.sessions, id)
	if err := manager.store.Delete(ctx, id); err != nil {
		return err
	}
	if manager.ledger != nil {
		// Deleted sessions keep counting toward monthly totals.
		if err := manager.ledger.Tombstone(id, time.Now()); err != nil {
			diagnostics.Warn("usage tombstone failed", err, "session_id", id)
		}
	}
	return nil
}

func (manager *Manager) Stop(ctx context.Context, id string) error {
	current := manager.get(id)
	if current == nil || !current.isRunning() {
		return nil
	}
	return manager.stop(ctx, current)
}

func (manager *Manager) StopAll(ctx context.Context) error {
	manager.shutdownMu.Lock()
	defer manager.shutdownMu.Unlock()
	manager.mu.Lock()
	manager.closed = true
	sessions := make([]*managedSession, 0, len(manager.sessions))
	for _, current := range manager.sessions {
		if current.isRunning() {
			sessions = append(sessions, current)
		}
	}
	manager.mu.Unlock()

	var wait sync.WaitGroup
	errorsOut := make(chan error, len(sessions))
	for _, current := range sessions {
		wait.Add(1)
		go func(current *managedSession) {
			defer wait.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					err := fmt.Errorf("stop session worker panic: %v", recovered)
					diagnostics.Error("session shutdown worker panicked", err, "session_id", current.config.ID, "stack", string(debug.Stack()))
					errorsOut <- err
				}
			}()
			if err := manager.stop(ctx, current); err != nil {
				errorsOut <- err
			}
		}(current)
	}
	wait.Wait()
	close(errorsOut)
	var stopErr error
	for err := range errorsOut {
		stopErr = errors.Join(stopErr, err)
	}
	if stopErr != nil {
		manager.mu.Lock()
		manager.closed = false
		manager.mu.Unlock()
	}
	return stopErr
}

func (manager *Manager) Close(ctx context.Context) error {
	stopErr := manager.StopAll(ctx)
	if stopErr != nil {
		return stopErr
	}
	manager.mu.Lock()
	if manager.storeClosed {
		manager.mu.Unlock()
		return nil
	}
	manager.closed = true
	closeErr := manager.closeStore()
	if closeErr == nil {
		manager.storeClosed = true
		closeErr = manager.closeUsage()
	}
	manager.mu.Unlock()
	return closeErr
}

func (manager *Manager) stop(ctx context.Context, current *managedSession) error {
	current.stopMu.Lock()
	defer current.stopMu.Unlock()
	if !current.isRunning() {
		return nil
	}
	if current.gracefulExit(ctx) {
		return nil
	}
	if !current.isRunning() {
		return nil
	}
	if err := current.process.Interrupt(); err != nil {
		if !current.isRunning() {
			return nil
		}
		current.markForced()
		if terminateErr := current.process.Terminate(); terminateErr != nil {
			return errors.Join(fmt.Errorf("interrupt session %q: %w", current.config.ID, err),
				fmt.Errorf("force-terminate session %q: %w", current.config.ID, terminateErr))
		}
		return waitForSession(ctx, current, terminateWait)
	}
	if waitErr := waitForSession(ctx, current, interruptWait); waitErr == nil {
		return nil
	} else if ctx.Err() != nil {
		return waitErr
	}
	if !current.isRunning() {
		return nil
	}
	current.markForced()
	if err := current.process.Terminate(); err != nil {
		return fmt.Errorf("force-terminate session %q: %w", current.config.ID, err)
	}
	return waitForSession(ctx, current, terminateWait)
}

// gracefulExit asks the harness to quit by itself and reports whether the
// session ended. A false result means the caller must fall back to interrupt
// and forced termination.
func (current *managedSession) gracefulExit(ctx context.Context) bool {
	adapter, err := harness.For(current.config.Tool)
	if err != nil {
		return false
	}
	steps := adapter.GracefulExit()
	if len(steps) == 0 {
		return false
	}
	for _, step := range steps {
		if err := current.process.Write([]byte(step.Input)); err != nil {
			diagnostics.Warn("graceful exit input failed", err, "session_id", current.config.ID)
			return false
		}
		if step.Pause > 0 {
			select {
			case <-time.After(step.Pause):
			case <-current.done:
				return true
			case <-ctx.Done():
				return false
			}
		}
	}
	return waitForSession(ctx, current, gracefulExitWait) == nil
}

func waitForSession(ctx context.Context, current *managedSession, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-current.done:
		return nil
	case <-timer.C:
		return fmt.Errorf("session %q did not stop within %s", current.config.ID, timeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (manager *Manager) supervise(current *managedSession) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("session supervisor panicked: %v", recovered)
			diagnostics.Error("session supervisor panicked", panicErr,
				"session_id", current.config.ID,
				"stack", string(debug.Stack()),
			)
			_ = current.process.Terminate()
			current.process.CloseConsole()
			closeErr := current.process.Close()
			detail := errors.Join(panicErr, closeErr).Error()
			if finishErr := manager.store.Finish(context.Background(), current.config.ID, session.StatusFailed, detail); finishErr != nil {
				detail = errors.Join(errors.New(detail), finishErr).Error()
				diagnostics.Error("persist session supervisor panic failed", finishErr, "session_id", current.config.ID)
			}
			current.mu.Lock()
			current.status = session.StatusFailed
			current.detail = detail
			current.mu.Unlock()
			select {
			case <-current.done:
			default:
				close(current.done)
			}
			manager.publish(Event{Kind: EventState, SessionID: current.config.ID, Session: current.snapshot()})
		}
	}()
	outputDone := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("session output relay panicked: %v", recovered)
				diagnostics.Error("session output relay panicked", err,
					"session_id", current.config.ID,
					"stack", string(debug.Stack()),
				)
				outputDone <- err
			}
		}()
		_, err := io.Copy(sessionOutputWriter{manager: manager, id: current.config.ID}, current.process.Output())
		outputDone <- err
	}()

	var exitCode uint32
	var waitErr error
	outputRead := false
	for {
		exited, code, err := current.process.Wait(100 * time.Millisecond)
		if err != nil {
			waitErr = fmt.Errorf("wait for session %q: %w", current.config.ID, err)
			break
		}
		if exited {
			exitCode = code
			break
		}
		select {
		case err := <-outputDone:
			outputRead = true
			if err == nil {
				waitErr = errors.New("ConPTY output closed while PowerShell was still running")
			} else if !errors.Is(err, os.ErrClosed) {
				waitErr = fmt.Errorf("read session %q output: %w", current.config.ID, err)
			}
			if terminateErr := current.process.Terminate(); terminateErr != nil {
				waitErr = errors.Join(waitErr, terminateErr)
			}
		default:
		}
		if waitErr != nil {
			break
		}
	}
	if waitErr != nil {
		if err := current.process.Terminate(); err != nil {
			waitErr = errors.Join(waitErr, fmt.Errorf("terminate failed session %q: %w", current.config.ID, err))
		}
		exited, code, err := current.process.Wait(terminateWait)
		if err != nil {
			waitErr = errors.Join(waitErr, err)
		} else if exited {
			exitCode = code
		} else {
			waitErr = errors.Join(waitErr, fmt.Errorf("session %q did not exit after termination", current.config.ID))
		}
	}
	current.process.CloseConsole()
	if !outputRead {
		if err := <-outputDone; err != nil && !errors.Is(err, os.ErrClosed) {
			waitErr = errors.Join(waitErr, fmt.Errorf("read session %q output: %w", current.config.ID, err))
		}
	}
	closeErr := current.process.Close()
	status, detail := outcome(exitCode, current.wasForced(), waitErr, closeErr)
	if err := manager.store.Finish(context.Background(), current.config.ID, status, detail); err != nil {
		status = session.StatusFailed
		detail = errors.Join(optionalError(detail), err).Error()
	}
	current.mu.Lock()
	current.status = status
	current.detail = detail
	current.mu.Unlock()
	close(current.done)
	if status == session.StatusFailed {
		diagnostics.Error("session process failed", errors.New(detail),
			"session_id", current.config.ID,
			"exit_code", exitCode,
			"forced", current.wasForced(),
		)
	} else {
		diagnostics.Info("session process exited",
			"session_id", current.config.ID,
			"status", status,
			"exit_code", exitCode,
			"forced", current.wasForced(),
		)
	}
	manager.publish(Event{Kind: EventState, SessionID: current.config.ID, Session: current.snapshot()})
}

type sessionOutputWriter struct {
	manager *Manager
	id      string
}

func (writer sessionOutputWriter) Write(data []byte) (int, error) {
	writer.manager.publish(Event{
		Kind:       EventOutput,
		SessionID:  writer.id,
		DataBase64: base64.StdEncoding.EncodeToString(data),
	})
	return len(data), nil
}

func (manager *Manager) recoverRuntime(ctx context.Context, id string) (session.Runtime, error) {
	for attempt := 0; attempt < 3; attempt++ {
		runtime, err := manager.store.Runtime(ctx, id)
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
		updated, err := manager.store.FailIfCurrent(ctx, runtime, detail)
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

func (manager *Manager) get(id string) *managedSession {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.sessions[id]
}

func (manager *Manager) publish(event Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("session event publication panicked", fmt.Errorf("%v", recovered),
				"session_id", event.SessionID,
				"event_kind", event.Kind,
				"stack", string(debug.Stack()),
			)
		}
	}()
	if manager.emit != nil {
		manager.emit(event)
	}
}

func (current *managedSession) snapshot() Snapshot {
	current.mu.RLock()
	defer current.mu.RUnlock()
	return Snapshot{
		ID:          current.config.ID,
		Name:        current.config.Name,
		Color:       current.config.Color,
		AutoResume:  current.config.AutoResume,
		Tool:        current.config.Tool,
		WorkingDir:  current.config.WorkingDir,
		Status:      current.status,
		LastError:   current.detail,
		RunningHere: current.status == session.StatusRunning,
		HasResumeID: current.hasResumeID,
	}
}

func (current *managedSession) isRunning() bool {
	current.mu.RLock()
	defer current.mu.RUnlock()
	return current.status == session.StatusRunning || current.status == session.StatusStarting
}

func (current *managedSession) markForced() {
	current.mu.Lock()
	current.forced = true
	current.mu.Unlock()
}

func (current *managedSession) wasForced() bool {
	current.mu.RLock()
	defer current.mu.RUnlock()
	return current.forced
}

func outcome(exitCode uint32, forced bool, runErr, closeErr error) (session.RuntimeStatus, string) {
	switch {
	case runErr != nil:
		return session.StatusFailed, errors.Join(runErr, closeErr).Error()
	case closeErr != nil:
		return session.StatusFailed, closeErr.Error()
	case forced:
		return session.StatusStopped, "Graceful interrupt failed or timed out; session process tree was force-terminated."
	case exitCode != 0:
		return session.StatusFailed, fmt.Sprintf("PowerShell exited with code %d", exitCode)
	default:
		return session.StatusStopped, ""
	}
}

func optionalError(detail string) error {
	if detail == "" {
		return nil
	}
	return errors.New(detail)
}
