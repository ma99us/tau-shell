//go:build windows

package sessionhost

import (
	"context"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
)

type fixedLatestFinder struct{ id string }

func (finder fixedLatestFinder) LatestSessionIn(string, string, time.Time, map[string]bool) (string, error) {
	return finder.id, nil
}

var _ harness.LatestSessionFinder = fixedLatestFinder{}

func TestTrackConversationsAdoptsTheHarnessNewConversationOnExit(t *testing.T) {
	ctx := context.Background()
	store, err := session.OpenStore(t.TempDir() + `\state.db`)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := &Manager{store: store, emit: func(Event) {}}
	config := session.Config{ID: "target", Tool: session.ClaudeCode}
	launch, err := store.Begin(ctx, config.ID, config.Tool, false)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	close(closed)

	stale := &managedSession{config: config, done: closed}
	manager.trackConversations(stale, fixedLatestFinder{id: "other-launch"}, "", time.Now(), "stale-token")
	if runtime, _ := store.Runtime(ctx, config.ID); runtime.HarnessSessionID != launch.HarnessSessionID {
		t.Fatalf("a stale launch changed the conversation to %q", runtime.HarnessSessionID)
	}

	current := &managedSession{config: config, done: closed}
	manager.trackConversations(current, fixedLatestFinder{id: "after-clear"}, "", time.Now(), launch.LaunchToken)
	if runtime, _ := store.Runtime(ctx, config.ID); runtime.HarnessSessionID != "after-clear" {
		t.Fatalf("conversation = %q, want after-clear", runtime.HarnessSessionID)
	}
}
