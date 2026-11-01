//go:build windows

package sessionhost

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/harness"
	"github.com/ma99us/tau-shell/internal/session"
)

type fixedDiscoveryAdapter struct {
	harness.Adapter
	id string
}

func (adapter fixedDiscoveryAdapter) DiscoverSessionID(string, time.Time, map[string]bool) (string, error) {
	return adapter.id, nil
}

func (fixedDiscoveryAdapter) ID() session.Harness { return session.Codex }

func TestDesktopDiscoveryRejectsStaleLaunchAndReportsClaimConflict(t *testing.T) {
	ctx := context.Background()
	store, err := session.OpenStore(t.TempDir() + `\state.db`)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var events []Event
	manager := &Manager{store: store, emit: func(event Event) { events = append(events, event) }}
	config := session.Config{ID: "target", Tool: session.Codex}
	closed := make(chan struct{})
	close(closed)

	oldLaunch, err := store.BeginDiscovered(ctx, config.ID, config.Tool, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, config.ID, session.StatusStopped, ""); err != nil {
		t.Fatal(err)
	}
	currentLaunch, err := store.BeginDiscovered(ctx, config.ID, config.Tool, false, false)
	if err != nil {
		t.Fatal(err)
	}
	stale := &managedSession{config: config, done: closed}
	manager.discoverSessionID(stale, fixedDiscoveryAdapter{id: "stale-id"}, time.Now(), nil, oldLaunch.LaunchToken)
	runtime, err := store.Runtime(ctx, config.ID)
	if err != nil || runtime.HarnessSessionID != "" || runtime.LaunchToken != currentLaunch.LaunchToken {
		t.Fatalf("stale discovery changed current runtime: %+v, %v", runtime, err)
	}
	if stale.hasResumeID {
		t.Fatal("stale discovery reported a resume ID")
	}

	owner, err := store.BeginDiscovered(ctx, "owner", config.Tool, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.ClaimHarnessSessionID(ctx, "owner", config.Tool, owner.LaunchToken, "claimed-id"); err != nil || !claimed {
		t.Fatalf("seed ID claim = %v, %v", claimed, err)
	}
	conflicted := &managedSession{config: config, done: closed}
	manager.discoverSessionID(conflicted, fixedDiscoveryAdapter{id: "claimed-id"}, time.Now(), nil, currentLaunch.LaunchToken)
	if !strings.Contains(conflicted.resumeNote, "already linked to another TauShell session") || conflicted.hasResumeID {
		t.Fatalf("desktop conflict state: resumeID=%t note=%q", conflicted.hasResumeID, conflicted.resumeNote)
	}
	if len(events) != 1 || events[0].Kind != EventState || !strings.Contains(events[0].Session.ResumeNote, "already linked to another TauShell session") {
		t.Fatalf("desktop conflict event = %+v", events)
	}
}
