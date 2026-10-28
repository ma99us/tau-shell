//go:build windows

package main

import (
	"context"
	"sync"
	"testing"

	"github.com/ma99us/tau-shell/internal/sessionhost"
)

func TestTrackShutdownProgressReportsEachSessionAndFinishes(t *testing.T) {
	manager := &closeTestManager{list: func(context.Context) ([]sessionhost.Snapshot, error) {
		return []sessionhost.Snapshot{
			{Name: "A", LastError: "force-terminated"},
			{Name: "B", RunningHere: true},
			{Name: "Other", RunningHere: true},
		}, nil
	}}
	var mu sync.Mutex
	var events []ShutdownProgress
	application := &App{manager: manager, emitEvent: func(name string, data ...any) {
		if name != "shutdown-progress" {
			t.Errorf("event name = %q", name)
		}
		mu.Lock()
		events = append(events, data[0].(ShutdownProgress))
		mu.Unlock()
	}}

	finish := application.trackShutdownProgress([]string{"A", "B"})
	finish()

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 2 || events[0].Phase != "stopping" || events[len(events)-1].Phase != "done" {
		t.Fatalf("events = %+v, want stopping first and done last", events)
	}
	got := events[0].Sessions
	if len(got) != 2 || got[0] != (ShutdownSession{"A", "force-stopped"}) || got[1] != (ShutdownSession{"B", "stopping"}) {
		t.Fatalf("sessions = %+v, want A force-stopped and B stopping only", got)
	}
}
