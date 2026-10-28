package sessionhost

import (
	"context"
	"errors"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
)

const discoverInterval = 2 * time.Second

// discoverSessionID learns the conversation ID of a harness that picks its
// own (Codex, OpenCode). It polls while the process runs and once more after
// it exits, never claiming an ID another TauShell session already owns. When
// several unclaimed conversations match, it records nothing and says so.
func (manager *Manager) discoverSessionID(current *managedSession, adapter harness.Adapter, launchedAt time.Time) {
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("session ID discovery panicked", errors.New("panic"), "session_id", current.config.ID)
		}
	}()
	config := current.config
	ctx := context.Background()
	lastProblem := ""
	attempt := func() bool {
		taken, err := manager.store.HarnessSessionIDs(ctx, config.Tool)
		if err != nil {
			diagnostics.Warn("read claimed harness session IDs failed", err, "session_id", config.ID)
			return false
		}
		found, err := adapter.DiscoverSessionID(config.WorkingDir, launchedAt, taken)
		switch {
		case errors.Is(err, harness.ErrAmbiguousSession):
			current.setResumeNote("Resume ID unknown: several " + string(config.Tool) + " conversations in this folder match; use Start fresh.")
			if lastProblem != err.Error() {
				diagnostics.Warn("harness session ID is ambiguous", err, "session_id", config.ID, "tool", config.Tool)
				lastProblem = err.Error()
			}
			return false
		case err != nil:
			if lastProblem != err.Error() {
				diagnostics.Warn("harness session ID discovery failed", err, "session_id", config.ID, "tool", config.Tool)
				lastProblem = err.Error()
			}
			return false
		case found == "":
			return false
		}
		recorded, err := manager.store.SetHarnessSessionID(ctx, config.ID, found)
		if err != nil {
			diagnostics.Warn("record harness session ID failed", err, "session_id", config.ID)
			return false
		}
		if !recorded {
			return true
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

func (current *managedSession) setResumeNote(note string) {
	current.mu.Lock()
	current.resumeNote = note
	current.mu.Unlock()
}
