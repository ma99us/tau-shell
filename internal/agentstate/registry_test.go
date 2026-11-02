package agentstate

import (
	"testing"
	"time"
)

func fixed() (*Registry, *time.Time) {
	clock := time.Unix(1000, 0)
	registry := NewRegistry()
	registry.now = func() time.Time { return clock }
	return registry, &clock
}

func TestIdleNeedsToHoldBeforeItIsConfirmed(t *testing.T) {
	registry, clock := fixed()
	registry.Report("a", Idle, Cue, false)
	if view := registry.Get("a"); view.State != Idle || view.Confirmed {
		t.Fatalf("a fresh idle must not be confirmed yet: %+v", view)
	}
	*clock = clock.Add(IdleFor)
	registry.Report("a", Idle, Cue, false)
	if view := registry.Get("a"); !view.Confirmed {
		t.Fatalf("idle that held must be confirmed: %+v", view)
	}
	registry.Report("a", Working, Cue, false)
	registry.Report("a", Idle, Cue, false)
	if view := registry.Get("a"); view.Confirmed {
		t.Fatalf("a flicker through working restarts the hold: %+v", view)
	}
}

func TestHeuristicStaleAndUnknownAreNeverConfirmed(t *testing.T) {
	registry, clock := fixed()
	registry.Report("h", Idle, Heuristic, false)
	registry.Report("w", Working, Cue, false)
	registry.Report("u", Unknown, Cue, false)
	*clock = clock.Add(IdleFor)
	if view := registry.Get("h"); view.Confirmed {
		t.Fatalf("heuristic idle must not be confirmed: %+v", view)
	}
	if view := registry.Get("w"); !view.Confirmed || view.State != Working {
		t.Fatalf("cue working is confirmed: %+v", view)
	}
	if view := registry.Get("u"); view.Confirmed {
		t.Fatalf("unknown must not be confirmed: %+v", view)
	}
	*clock = clock.Add(FreshFor + time.Second)
	if view := registry.Get("w"); view.State != Unknown || view.Confirmed {
		t.Fatalf("a stale report becomes unknown: %+v", view)
	}
	if view := registry.Get("never"); view.State != Unknown || view.Confirmed {
		t.Fatalf("a session that never reported is unknown: %+v", view)
	}
}

func TestValidateAndForget(t *testing.T) {
	if Valid("stopped", Cue) || Valid(Idle, "guess") || !Valid(Input, Cue) {
		t.Fatal("validation is wrong")
	}
	registry, _ := fixed()
	registry.Report("a", Input, Cue, true)
	if view := registry.Get("a"); !view.HasDraft || !view.Confirmed {
		t.Fatalf("%+v", view)
	}
	registry.Forget("a")
	if registry.Get("a").State != Unknown {
		t.Fatal("forgotten session must be unknown")
	}
}
