package models

import (
	"context"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
)

const (
	// ProbeInterval is how long a successful probe result is reused.
	ProbeInterval = 24 * time.Hour
	// failedProbeInterval keeps a failing probe from spawning the harness on
	// every menu open.
	failedProbeInterval = time.Hour
	probeTimeout        = 30 * time.Second
)

type cached struct {
	result  Result
	at      time.Time
	failed  bool
	message string
}

// Service runs harness probes in the background and remembers the results for
// a day. Get never blocks: it returns what is known now and starts a refresh
// when the answer is missing or old.
type Service struct {
	env      Env
	now      func() time.Time
	onUpdate func(harnessID string)

	mu      sync.Mutex
	results map[string]cached
	running map[string]bool
}

// NewService creates a probe cache. onUpdate runs after a probe finishes so the
// UI can reload the menu; it may be nil.
func NewService(env Env, onUpdate func(harnessID string)) *Service {
	return &Service{env: env, now: time.Now, onUpdate: onUpdate, results: map[string]cached{}, running: map[string]bool{}}
}

// Status describes the probe state for one harness.
type Status struct {
	Result Result
	// Probing is true while a probe is running.
	Probing bool
	// Error is the latest probe failure; empty when the last probe worked.
	Error string
}

// Get returns the cached probe result and starts a refresh when it is due.
// The harness executable is resolved inside the background probe.
func (service *Service) Get(ctx context.Context, harnessID string, force bool) Status {
	if !HasProbe(harnessID) {
		return Status{}
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	entry, known := service.results[harnessID]
	maxAge := ProbeInterval
	if entry.failed {
		maxAge = failedProbeInterval
	}
	due := !known || force || service.now().Sub(entry.at) >= maxAge
	if due && !service.running[harnessID] {
		service.running[harnessID] = true
		go service.run(ctx, harnessID)
	}
	return Status{Result: entry.result, Probing: service.running[harnessID], Error: entry.message}
}

func (service *Service) run(parent context.Context, harnessID string) {
	ctx, cancel := context.WithTimeout(parent, probeTimeout)
	defer cancel()
	executable := ""
	if service.env.Executable != nil {
		executable = service.env.Executable(ctx, harnessID)
	}
	result, err := Probe(ctx, harnessID, executable, service.env)
	service.mu.Lock()
	previous := service.results[harnessID]
	entry := cached{result: result, at: service.now()}
	if err != nil {
		// Keep the last good result when a later probe fails.
		entry = cached{result: previous.result, at: service.now(), failed: true, message: err.Error()}
		if !previous.failed {
			diagnostics.Warn("model probe failed; using the manifest list", err, "harness", harnessID)
		}
	}
	service.results[harnessID] = entry
	delete(service.running, harnessID)
	service.mu.Unlock()
	if service.onUpdate != nil && parent.Err() == nil {
		service.onUpdate(harnessID)
	}
}
