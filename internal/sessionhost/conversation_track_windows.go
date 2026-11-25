package sessionhost

import (
	"context"
	"errors"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/harness"
)

const conversationPollInterval = 5 * time.Second

// trackConversations notices a conversation the harness started on its own
// (/clear, /new) and records it as the session's current one, so a plain
// Resume continues the latest conversation and History lists it. It polls
// while the process runs and once more after it exits. Ambiguous or claimed
// candidates are never adopted.
func (manager *Manager) trackConversations(current *managedSession, finder harness.LatestSessionFinder, home string, launchedAt time.Time, launchToken string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnostics.Error("conversation tracking panicked", errors.New("panic"), "session_id", current.config.ID)
		}
	}()
	config := current.config
	ctx := context.Background()
	lastProblem := ""
	attempt := func() {
		if current.switching.Load() {
			return
		}
		runtime, err := manager.store.Runtime(ctx, config.ID)
		// Discovery owns the first ID; a changed launch token means a newer launch.
		if err != nil || runtime.LaunchToken != launchToken || runtime.HarnessSessionID == "" {
			return
		}
		taken, err := manager.store.HarnessSessionIDs(ctx, config.Tool)
		if err != nil {
			return
		}
		found, err := finder.LatestSessionIn(home, config.WorkingDir, launchedAt, taken)
		if err != nil {
			if lastProblem != err.Error() {
				diagnostics.Warn("latest harness conversation is ambiguous", err, "session_id", config.ID, "tool", config.Tool)
				lastProblem = err.Error()
			}
			return
		}
		if found == "" || found == runtime.HarnessSessionID {
			return
		}
		// Record the outgoing conversation's usage before the ID moves on.
		manager.ingestSession(ctx, config.ID, true)
		if _, err := manager.store.AdoptHarnessSessionID(ctx, config.ID, config.Tool, launchToken, runtime.HarnessSessionID, found); err != nil {
			if lastProblem != err.Error() {
				diagnostics.Warn("could not adopt the harness's new conversation", err, "session_id", config.ID, "tool", config.Tool)
				lastProblem = err.Error()
			}
			return
		}
		diagnostics.Info("harness started a new conversation", "session_id", config.ID, "tool", config.Tool, "harness_session_id", found)
		manager.ingestSession(ctx, config.ID, true)
		manager.publish(Event{Kind: EventState, SessionID: config.ID, Session: current.snapshot()})
	}

	ticker := time.NewTicker(conversationPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-current.done:
			attempt()
			return
		case <-ticker.C:
			attempt()
		}
	}
}

// startConversationTracking begins tracking when the harness can report its
// newest conversation; harnesses without a reliable signal are skipped.
func (manager *Manager) startConversationTracking(current *managedSession, adapter harness.Adapter, home string, launchedAt time.Time, launchToken string) {
	finder, ok := adapter.(harness.LatestSessionFinder)
	if !ok {
		return
	}
	go manager.trackConversations(current, finder, home, launchedAt, launchToken)
}
