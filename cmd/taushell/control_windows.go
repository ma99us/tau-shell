package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ma99us/tau-shell/internal/agentstate"
	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/control"
	"github.com/ma99us/tau-shell/internal/diagnostics"
)

const controlAuditLimit = 200

// controlHandler serves control requests from the running app.
type controlHandler struct {
	startedAt time.Time
	sessions  control.Sessions
}

func (handler controlHandler) Handle(_ control.Caller, request control.Request) control.Response {
	switch request.Op {
	case control.OpHello:
		return control.OK(control.Hello{
			Version:   control.ProtocolVersion,
			PID:       os.Getpid(),
			StartedAt: handler.startedAt.UTC().Format(time.RFC3339Nano),
			App:       "taushell " + version,
		})
	case control.OpList:
		return handler.sessions.HandleList()
	case control.OpWait:
		return handler.sessions.HandleWait(request.Args)
	case control.OpSend:
		return handler.sessions.HandleSend(request.Args)
	case control.OpStart:
		return handler.sessions.HandleStart(request.Args)
	case control.OpStop:
		return handler.sessions.HandleStop(request.Args)
	case control.OpRead:
		return handler.sessions.HandleScreen(request.Args)
	default:
		return control.Failure(control.CodeUnknownOp, "unknown operation")
	}
}

func (application *App) agentStates() *agentstate.Registry {
	application.statesOnce.Do(func() { application.states = agentstate.NewRegistry() })
	return application.states
}

// ReportSessionState is called by the window about once a second per running
// session. Only the state is sent, never terminal content. question is true
// when a waiting harness shows a question rather than a permission prompt.
func (application *App) ReportSessionState(id, state, source string, hasDraft, question bool) error {
	if !agentstate.Valid(agentstate.State(state), agentstate.Source(source)) {
		return fmt.Errorf("invalid session state %q from %q", state, source)
	}
	states := application.agentStates()
	states.Report(id, agentstate.State(state), agentstate.Source(source), hasDraft)
	if question {
		states.MarkQuestion(id)
	}
	return nil
}

// screenWaiters maps a request id to the channel waiting for the window's answer.
var screenWaiters sync.Map

var errScreenOff = errors.New("screen sharing is off on the PC (mobile.screen: false)")

// requestScreen asks the window, which owns the terminal, for the last rows of
// a session. The text is handed straight to the caller; it is never stored or logged.
func (application *App) requestScreen(id string, rows int) ([]string, error) {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil || !settings.Mobile.ShareScreen() {
		return nil, errScreenOff
	}
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	request := hex.EncodeToString(token)
	answer := make(chan []string, 1)
	screenWaiters.Store(request, answer)
	defer screenWaiters.Delete(request)
	application.emit("control-screen", map[string]any{"request": request, "id": id, "rows": rows})
	select {
	case text := <-answer:
		return text, nil
	case <-time.After(3 * time.Second):
		return nil, errors.New("the window did not answer in time")
	}
}

// ProvideScreen is called by the window with the rows of a control-screen request.
func (application *App) ProvideScreen(request string, rows []string) error {
	waiter, ok := screenWaiters.Load(request)
	if !ok {
		return errors.New("no such screen request")
	}
	select {
	case waiter.(chan []string) <- rows:
	default:
	}
	return nil
}

// SetQueuedPrompts tells the app how many prompts are waiting in the window's queues,
// so that closing can warn that they are discarded.
func (application *App) SetQueuedPrompts(count int) {
	if count < 0 {
		count = 0
	}
	atomic.StoreInt32(&application.queuedPrompts, int32(count))
}

// RecordQueueSend notes that the queue typed a prompt into a session. It records the
// session and outcome only, never the prompt text.
func (application *App) RecordQueueSend(session, outcome string) {
	diagnostics.Info("queued prompt sent", "session_id", session, "outcome", outcome)
	application.controlMu.Lock()
	audit := application.controlAudit
	application.controlMu.Unlock()
	if audit != nil {
		audit.Add(control.AuditEntry{Time: time.Now(), Image: "prompt queue", Op: "queue send", Session: session, Outcome: outcome})
	}
}

func (application *App) controlSessions() control.Sessions {
	return control.Sessions{
		List: func(ctx context.Context) ([]control.SessionInfo, error) {
			snapshots, err := application.manager.List(ctx)
			if err != nil {
				return nil, err
			}
			usageSummary, usageErr := application.manager.UsageSummary(ctx)
			if usageErr != nil {
				diagnostics.Warn("phone session usage unavailable", usageErr)
			}
			infos := make([]control.SessionInfo, 0, len(snapshots))
			for _, snapshot := range snapshots {
				sessionUsage := usageSummary.Sessions[snapshot.ID]
				infos = append(infos, control.SessionInfo{
					ID:          snapshot.ID,
					Name:        snapshot.Name,
					Tool:        string(snapshot.Tool),
					Status:      string(snapshot.Status),
					RunningHere: snapshot.RunningHere,
					WorkingDir:  snapshot.WorkingDir,
					Color:       snapshot.Color,
					Notes:       snapshot.Notes,
					Link:        snapshot.Link,
					LinkLabel:   snapshot.LinkLabel,
					Tags:        snapshot.Tags,
					LastError:   snapshot.LastError,
					TotalUSD:    sessionUsage.USD,
					Unpriced:    sessionUsage.Unpriced,
				})
			}
			return infos, nil
		},
		States: application.agentStates(),
		Launch: func(id, mode string) error {
			application.emit("control-launch", map[string]any{"id": id, "mode": mode})
			return nil
		},
		Screen: application.requestScreen,
		Stop: func(id string) error {
			// A graceful stop can outlast the pipe timeout, so it runs on its own.
			go func() { _ = application.StopSession(id) }()
			return nil
		},
		Answer: func(id, answer string) error {
			application.emit("control-answer", map[string]any{"id": id, "answer": answer})
			return nil
		},
		Deliver: func(id, text string, submit bool) error {
			// The window types through xterm so input accounting matches real typing.
			application.emit("control-send", map[string]any{"id": id, "text": text, "submit": submit})
			return nil
		},
		DeliverMobile: func(id, text string, submit bool) error {
			application.emit("control-send", map[string]any{"id": id, "text": text, "submit": submit, "allowBusySubmit": true})
			return nil
		},
	}
}

// startControl opens the control pipe when settings enable it at startup. The
// channel is off by default; a failure is reported to the UI, never swallowed.
func (application *App) startControl() {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil {
		// NotifyAttention already surfaces invalid settings; keep the channel off.
		diagnostics.Warn("settings invalid; control channel stays off", err)
		return
	}
	if !settings.Control.ChannelEnabled() {
		return
	}
	if err := application.applyControl(true); err != nil {
		application.emit("settings-error", err.Error())
	}
}

func (application *App) stopControl() {
	_ = application.applyControl(false)
}

// SetControlEnabled opens or closes the control pipe now and stores the choice
// in config.yaml (edited in place) so it is the state at the next startup.
func (application *App) SetControlEnabled(enabled bool) error {
	if err := application.applyControl(enabled); err != nil {
		return err
	}
	if err := appconfig.SetControlEnabled(application.settingsPath(), enabled); err != nil {
		diagnostics.Error("save control channel setting failed", err)
		return fmt.Errorf("The control channel is %s now, but the setting could not be saved: %w", map[bool]string{true: "on", false: "off"}[enabled], err)
	}
	return nil
}

func (application *App) applyControl(enabled bool) error {
	application.controlMu.Lock()
	defer application.controlMu.Unlock()
	if !enabled {
		if application.controlServer != nil {
			application.controlServer.Close()
			application.controlServer = nil
			diagnostics.Info("control channel closed")
		}
		return nil
	}
	if application.controlServer != nil {
		return nil
	}
	if application.controlAudit == nil {
		application.controlAudit = control.NewAuditLog(controlAuditLimit, func(entry control.AuditEntry) {
			application.emit("control-audit", entry)
		})
	}
	server, err := control.Listen(controlHandler{startedAt: time.Now(), sessions: application.controlSessions()}, application.controlAudit)
	if err != nil {
		diagnostics.Error("control channel failed to start", err)
		return fmt.Errorf("Control channel could not start: %w", err)
	}
	application.controlServer = server
	diagnostics.Info("control channel listening")
	return nil
}

// ControlStatus reports the channel state and the audit trail to the UI.
type ControlStatus struct {
	Enabled bool                 `json:"enabled"`
	Audit   []control.AuditEntry `json:"audit"`
}

func (application *App) GetControlStatus() ControlStatus {
	application.controlMu.Lock()
	defer application.controlMu.Unlock()
	status := ControlStatus{Enabled: application.controlServer != nil, Audit: []control.AuditEntry{}}
	if application.controlAudit != nil {
		status.Audit = application.controlAudit.Entries()
	}
	return status
}
