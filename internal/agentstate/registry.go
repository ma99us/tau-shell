// Package agentstate keeps the authoritative, honest view of what each running
// session is doing, for consumers that act on it (control channel, later
// dispatch and mobile). It stores states and how they were obtained, never
// terminal content.
package agentstate

import (
	"sync"
	"time"
)

// State is what a session is doing right now.
type State string

const (
	Working State = "working"
	// Input means the harness is waiting for the user to answer a prompt
	// (permission request, picker). Text must never be typed into it.
	Input   State = "input"
	Idle    State = "idle"
	Unknown State = "unknown"
	Stopped State = "stopped"
)

// Source says how a state was obtained.
type Source string

const (
	// Cue: the harness itself showed it (title spinner, status row, prompt footer).
	Cue Source = "cue"
	// Heuristic: inferred, for example from output timing.
	Heuristic Source = "heuristic"
)

const (
	// FreshFor is how long a report stays trustworthy. The UI reports about
	// once a second; silence means nobody is watching the session any more.
	FreshFor = 4 * time.Second
	// IdleFor is how long idle must hold before it counts as confirmed, so a
	// pause between spinner frames is not mistaken for a finished turn.
	IdleFor = 1500 * time.Millisecond
)

// View is the answer to "what is this session doing".
type View struct {
	State State `json:"state"`
	// Confirmed is true only for a fresh, cue-based state (and, for idle, one
	// that has held for IdleFor). Automation must require it.
	Confirmed bool   `json:"confirmed"`
	Source    Source `json:"source,omitempty"`
	// HasDraft reports that text is already typed in the harness input.
	HasDraft bool `json:"hasDraft,omitempty"`
	// AgeMS is the time since the last report; SinceMS since the state began.
	AgeMS   int64 `json:"ageMs"`
	SinceMS int64 `json:"sinceMs"`
}

type entry struct {
	state    State
	source   Source
	hasDraft bool
	since    time.Time
	reported time.Time
}

// Registry is safe for concurrent use.
type Registry struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]entry
}

func NewRegistry() *Registry { return &Registry{now: time.Now, entries: map[string]entry{}} }

// Valid reports whether the values are ones the registry accepts.
func Valid(state State, source Source) bool {
	switch state {
	case Working, Input, Idle, Unknown:
	default:
		return false
	}
	return source == Cue || source == Heuristic
}

// Report records the latest observation for a running session.
func (registry *Registry) Report(id string, state State, source Source, hasDraft bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	now := registry.now()
	current, ok := registry.entries[id]
	since := current.since
	if !ok || current.state != state {
		since = now
	}
	registry.entries[id] = entry{state: state, source: source, hasDraft: hasDraft, since: since, reported: now}
}

// Forget drops a session's state, for example when it stops.
func (registry *Registry) Forget(id string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	delete(registry.entries, id)
}

// Get returns the current view. A session that never reported, or whose report
// is stale, is Unknown and never confirmed.
func (registry *Registry) Get(id string) View {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	now := registry.now()
	current, ok := registry.entries[id]
	if !ok {
		return View{State: Unknown}
	}
	age := now.Sub(current.reported)
	view := View{State: current.state, Source: current.source, HasDraft: current.hasDraft, AgeMS: age.Milliseconds(), SinceMS: now.Sub(current.since).Milliseconds()}
	if age > FreshFor {
		view.State, view.Source = Unknown, ""
		return view
	}
	if current.source != Cue || current.state == Unknown {
		return view
	}
	view.Confirmed = current.state != Idle || now.Sub(current.since) >= IdleFor
	return view
}
