package pairing

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"sync"
	"time"
)

// Pairing failures. Each is visible to the user on the PC.
var (
	ErrNoWindow  = errors.New("no pairing window is open")
	ErrBadCode   = errors.New("wrong pairing code")
	ErrExpired   = errors.New("the pairing window expired")
	ErrDenied    = errors.New("pairing was denied on the PC")
	ErrBusy      = errors.New("a pairing request is already waiting for approval")
	ErrTooManyTries = errors.New("too many wrong codes; the pairing window is closed")
)

// MaxCodeAttempts is how many wrong codes close the window.
const MaxCodeAttempts = 5

// State of the pairing window.
type State string

const (
	StateIdle     State = "idle"
	StateOpen     State = "open"
	StateAwaiting State = "awaiting-approval"
)

// Outcome is what the waiting phone receives. Token is set only on success.
type Outcome struct {
	Device Device
	Token  string
	Err    error
}

// Pending is a phone that presented the right code and waits for the PC user.
type Pending struct {
	Name string
	done chan Outcome
}

// Wait blocks until the PC user decides, the window expires or ctx ends.
func (pending *Pending) Wait(ctx context.Context) (Outcome, error) {
	select {
	case outcome := <-pending.done:
		return outcome, nil
	case <-ctx.Done():
		return Outcome{}, ctx.Err()
	}
}

// Window is the single, short-lived, single-use pairing window. A phone needs
// the one-time code AND the PC user's approval of the displayed device name.
type Window struct {
	registry *Registry
	hosts    []string
	lifetime time.Duration
	clock    func() time.Time

	mu       sync.Mutex
	state    State
	code     string
	expires  time.Time
	attempts int
	pending  *Pending
}

// NewWindow creates a window that adds approved devices to registry with a
// certificate valid for the given hosts and lifetime.
func NewWindow(registry *Registry, hosts []string, lifetime time.Duration, clock func() time.Time) *Window {
	if clock == nil {
		clock = time.Now
	}
	return &Window{registry: registry, hosts: hosts, lifetime: lifetime, clock: clock, state: StateIdle}
}

const codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ" // no look-alike characters

func newCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for index := range raw {
		raw[index] = codeAlphabet[int(raw[index])%len(codeAlphabet)]
	}
	return string(raw), nil
}

// Open starts a window and returns the code to put in the QR. Opening again
// replaces any earlier window and cancels a request waiting for approval.
func (window *Window) Open(ttl time.Duration) (string, error) {
	code, err := newCode()
	if err != nil {
		return "", err
	}
	window.mu.Lock()
	defer window.mu.Unlock()
	window.cancelPendingLocked(ErrNoWindow)
	window.state, window.code, window.attempts = StateOpen, code, 0
	window.expires = window.clock().Add(ttl)
	return code, nil
}

// Close ends the window and cancels a request waiting for approval.
func (window *Window) Close() {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.cancelPendingLocked(ErrNoWindow)
	window.state, window.code = StateIdle, ""
}

// State reports the current state, closing an expired window first.
func (window *Window) State() State {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.expireLocked()
	return window.state
}

// PendingName is the device name awaiting approval, for the PC dialog.
func (window *Window) PendingName() (string, bool) {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.expireLocked()
	if window.pending == nil {
		return "", false
	}
	return window.pending.Name, true
}

func (window *Window) expireLocked() {
	if window.state != StateIdle && !window.clock().Before(window.expires) {
		window.cancelPendingLocked(ErrExpired)
		window.state, window.code = StateIdle, ""
	}
}

func (window *Window) cancelPendingLocked(reason error) {
	if window.pending != nil {
		window.pending.done <- Outcome{Err: reason}
		window.pending = nil
	}
}

// Submit is called by the phone. The code is consumed on success, so a second
// phone with the same code is refused. Wrong codes count; too many close the window.
func (window *Window) Submit(code, deviceName string) (*Pending, error) {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.expireLocked()
	switch window.state {
	case StateIdle:
		return nil, ErrNoWindow
	case StateAwaiting:
		return nil, ErrBusy
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(window.code)) != 1 {
		window.attempts++
		if window.attempts >= MaxCodeAttempts {
			window.state, window.code = StateIdle, ""
			return nil, ErrTooManyTries
		}
		return nil, ErrBadCode
	}
	if deviceName == "" || len(deviceName) > 64 {
		return nil, errors.New("device name must be 1-64 characters")
	}
	window.pending = &Pending{Name: deviceName, done: make(chan Outcome, 1)}
	window.state, window.code = StateAwaiting, ""
	return window.pending, nil
}

// Decide is called by the PC user. Approving creates the device and hands the
// phone its token once; denying or an expired window tells the phone why.
func (window *Window) Decide(approve bool) error {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.expireLocked()
	pending := window.pending
	if pending == nil {
		return ErrNoWindow
	}
	window.pending, window.state = nil, StateIdle
	if !approve {
		pending.done <- Outcome{Err: ErrDenied}
		return nil
	}
	device, token, err := window.registry.Add(pending.Name, window.hosts, window.lifetime)
	if err != nil {
		pending.done <- Outcome{Err: errors.New("the PC could not create the device")}
		return err
	}
	pending.done <- Outcome{Device: device, Token: token}
	return nil
}
