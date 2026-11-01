package sessionhost

import (
	"context"
	"errors"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
)

const discoverInterval = 2 * time.Second

// discoverSessionID learns the conversation ID of a harness that picks its
// own (Codex, OpenCode). It polls while the process runs and once more after
// it exits, never claiming an ID another TauShell session already owns. When
// several unclaimed conversations match, it records nothing and says so.
func (manager *Manager) discoverSessionID(current *managedSession, adapter harness.Adapter, launchedAt time.Time, existingIDs map[string]bool, launchToken string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("session ID discovery panicked", errors.New("panic"), "session_id", current.config.ID)
		}
	}()
	config := current.config
	ctx := context.Background()
	lastProblem := ""
	attempt := func() bool {
		found, err := harness.DiscoverAndClaimSessionID(ctx, manager.store, adapter, config.ID, config.WorkingDir, launchedAt, existingIDs, launchToken)
		switch {
		case errors.Is(err, harness.ErrAmbiguousSession):
			note := "Resume ID unknown: several " + string(config.Tool) + " conversations in this folder match; use Start fresh."
			if current.setResumeNote(note) {
				manager.publish(Event{Kind: EventState, SessionID: config.ID, Session: current.snapshot()})
			}
			if lastProblem != err.Error() {
				diagnostics.Warn("harness session ID is ambiguous", err, "session_id", config.ID, "tool", config.Tool)
				lastProblem = err.Error()
			}
			return false
		case errors.Is(err, session.ErrHarnessSessionIDClaimed):
			note := "Resume ID unknown: this " + string(config.Tool) + " conversation is already linked to another TauShell session."
			if current.setResumeNote(note) {
				manager.publish(Event{Kind: EventState, SessionID: config.ID, Session: current.snapshot()})
			}
			diagnostics.Warn("harness session ID is already claimed", err, "session_id", config.ID, "tool", config.Tool)
			return true
		case errors.Is(err, session.ErrHarnessLaunchChanged):
			diagnostics.Info("discarding stale harness session ID discovery", "session_id", config.ID, "tool", config.Tool)
			return true
		case err != nil:
			if lastProblem != err.Error() {
				diagnostics.Warn("harness session ID discovery failed", err, "session_id", config.ID, "tool", config.Tool)
				lastProblem = err.Error()
			}
			return false
		case found == "":
			return false
		}
		current.setResumeID()
		diagnostics.Info("harness session ID discovered", "session_id", config.ID, "tool", config.Tool, "harness_session_id", found)
		manager.publish(Event{Kind: EventState, SessionID: config.ID, Session: current.snapshot()})
		return true
	}

	ticker := time.NewTicker(discoverInterval)
	defer ticker.Stop()
	for {
		select {
		case <-current.done:
			if !attempt() {
				diagnostics.Warn("no resumable harness conversation was found for this launch", errors.New("not found"), "session_id", config.ID, "tool", config.Tool)
			}
			return
		case <-ticker.C:
			if attempt() {
				return
			}
		}
	}
}

func (current *managedSession) setResumeID() {
	current.mu.Lock()
	current.hasResumeID = true
	current.resumeNote = ""
	current.mu.Unlock()
}

func (current *managedSession) setResumeNote(note string) bool {
	current.mu.Lock()
	changed := current.resumeNote != note
	current.resumeNote = note
	current.mu.Unlock()
	return changed
}
