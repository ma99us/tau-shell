package main

import (
	"context"
	"fmt"
	"os"
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
	default:
		return control.Failure(control.CodeUnknownOp, "unknown operation")
	}
}

func (application *App) agentStates() *agentstate.Registry {
	application.statesOnce.Do(func() { application.states = agentstate.NewRegistry() })
	return application.states
}

// ReportSessionState is called by the window about once a second per running
// session. Only the state is sent, never terminal content.
func (application *App) ReportSessionState(id, state, source string, hasDraft bool) error {
	if !agentstate.Valid(agentstate.State(state), agentstate.Source(source)) {
		return fmt.Errorf("invalid session state %q from %q", state, source)
	}
	application.agentStates().Report(id, agentstate.State(state), agentstate.Source(source), hasDraft)
	return nil
}

func (application *App) controlSessions() control.Sessions {
	return control.Sessions{
		List: func(ctx context.Context) ([]control.SessionInfo, error) {
			snapshots, err := application.manager.List(ctx)
			if err != nil {
				return nil, err
			}
			infos := make([]control.SessionInfo, 0, len(snapshots))
			for _, snapshot := range snapshots {
				infos = append(infos, control.SessionInfo{
					ID:          snapshot.ID,
					Name:        snapshot.Name,
					Tool:        string(snapshot.Tool),
					Status:      string(snapshot.Status),
					RunningHere: snapshot.RunningHere,
					WorkingDir:  snapshot.WorkingDir,
					LastError:   snapshot.LastError,
				})
			}
			return infos, nil
		},
		States: application.agentStates(),
		Deliver: func(id, text string, submit bool) error {
			// The window types through xterm so input accounting matches real typing.
			application.emit("control-send", map[string]any{"id": id, "text": text, "submit": submit})
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

