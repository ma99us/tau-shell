package mobile

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/agentstate"
	"github.com/ma99us/tau-shell/internal/control"
	"github.com/ma99us/tau-shell/internal/pairing"
)

type plainSealer struct{}

func (plainSealer) Seal(plain []byte) ([]byte, error)  { return append([]byte("s:"), plain...), nil }
func (plainSealer) Open(sealed []byte) ([]byte, error) { return bytes.TrimPrefix(sealed, []byte("s:")), nil }

type rig struct {
	server   *Server
	address  string
	states   *agentstate.Registry
	mu       sync.Mutex
	audit    []string
	deliverd []string

	registryPath string
	listener     net.Listener
}

func newRig(t *testing.T) *rig {
	t.Helper()
	registryPath := t.TempDir() + "\\devices.json"
	registry, err := pairing.OpenRegistry(registryPath, plainSealer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state := &rig{states: agentstate.NewRegistry(), registryPath: registryPath}
	sessions := control.Sessions{
		List: func(context.Context) ([]control.SessionInfo, error) {
			return []control.SessionInfo{{ID: "s1", Name: "demo", RunningHere: true}}, nil
		},
		States: state.states,
		Deliver: func(id, text string, submit bool) error {
			state.mu.Lock()
			defer state.mu.Unlock()
			state.deliverd = append(state.deliverd, id)
			return nil
		},
		Launch: func(string, string) error { return nil },
		Screen: func(string, int) ([]string, error) { return []string{"hello", "", "world", "  "}, nil },
	}
	state.server = &Server{
		Registry: registry,
		Window:   pairing.NewWindow(registry, []string{"127.0.0.1"}, time.Hour, nil),
		Backend:  sessions,
		Audit: func(device, op, session, outcome string) {
			state.mu.Lock()
			defer state.mu.Unlock()
			state.audit = append(state.audit, op+" "+session+" "+outcome)
		},
		PairWait: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	state.listener = listener
	go state.server.Serve(listener)
	t.Cleanup(func() { listener.Close() })
	state.address = listener.Addr().String()
	return state
}

func (r *rig) call(t *testing.T, sni, fingerprint, method, path, token, body string) (int, string, error) {
	t.Helper()
	config := pairing.PinnedClientConfig(fingerprint)
	config.ServerName = sni
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: config, DisableKeepAlives: true}, Timeout: 10 * time.Second}
	request, _ := http.NewRequest(method, "https://"+r.address+path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(raw), nil
}

// pair runs the whole flow with a scripted phone and returns its credentials.
func (r *rig) pair(t *testing.T) PairResult {
	t.Helper()
	credentials, err := pairing.New([]string{"127.0.0.1"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	r.server.SetPairingCredentials(&credentials)
	code, _ := r.server.Window.Open(time.Minute)
	type reply struct {
		status int
		body   string
		err    error
	}
	done := make(chan reply, 1)
	go func() {
		status, body, err := r.call(t, PairName, credentials.Fingerprint, "POST", "/v1/pair", "", `{"code":"`+code+`","name":"Pixel"}`)
		done <- reply{status, body, err}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for r.server.Window.State() != pairing.StateAwaiting {
		if time.Now().After(deadline) {
			t.Fatal("the phone never reached the awaiting-approval state")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := r.server.Window.Decide(true); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.err != nil || result.status != 200 {
		t.Fatalf("pair: %d %v %s", result.status, result.err, result.body)
	}
	var paired PairResult
	if err := json.Unmarshal([]byte(result.body), &paired); err != nil {
		t.Fatal(err)
	}
	r.server.SetPairingCredentials(nil)
	return paired
}

func TestScreenReturnsRowsAndAuditHoldsNoText(t *testing.T) {
	r := newRig(t)
	device := r.pair(t)
	name := DeviceName(device.DeviceID)
	status, body, err := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions/s1/screen?rows=10", device.Token, "")
	if err != nil || status != 200 || !strings.Contains(body, `["hello","","world"]`) {
		t.Fatalf("screen: %d %v %s", status, err, body)
	}
	if status, _, _ = r.call(t, name, device.Fingerprint, "GET", "/v1/sessions/nope/screen", device.Token, ""); status != 404 {
		t.Fatalf("unknown session: %d", status)
	}
	if status, _, _ = r.call(t, name, device.Fingerprint, "GET", "/v1/sessions/s1/screen", "wrong", ""); status != 401 {
		t.Fatalf("screen needs the token: %d", status)
	}
	for _, line := range r.audit {
		if strings.Contains(line, "hello") || strings.Contains(line, "world") {
			t.Fatalf("audit must never hold terminal text: %q", line)
		}
	}
}

func TestPairingSurvivesServerRestart(t *testing.T) {
	r := newRig(t)
	device := r.pair(t)
	name := DeviceName(device.DeviceID)
	path := r.registryPath
	r.listener.Close()

	registry, err := pairing.OpenRegistry(path, plainSealer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.server.Registry = registry
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go r.server.Serve(listener)
	r.address = listener.Addr().String()
	status, body, err := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", device.Token, "")
	if err != nil || status != 200 {
		t.Fatalf("a paired phone must work after a restart: %d %v %s", status, err, body)
	}
}

func TestWritesAreRateLimitedPerDevice(t *testing.T) {
	r := newRig(t)
	device := r.pair(t)
	name := DeviceName(device.DeviceID)
	var last int
	for attempt := 0; attempt < maxWrites+3; attempt++ {
		last, _, _ = r.call(t, name, device.Fingerprint, "POST", "/v1/sessions/s1/send", device.Token, `{"text":"x","insertOnly":true}`)
	}
	if last != 429 {
		t.Fatalf("writes past the limit must get 429, got %d", last)
	}
	if status, _, _ := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", device.Token, ""); status != 200 {
		t.Fatalf("reads are not limited: %d", status)
	}
}

func TestPresetsComeFromThePCAndSkipBlanks(t *testing.T) {
	r := newRig(t)
	r.server.Presets = func() []string { return []string{"Run the tests", "  ", " Commit it "} }
	device := r.pair(t)
	status, body, err := r.call(t, DeviceName(device.DeviceID), device.Fingerprint, "GET", "/v1/presets", device.Token, "")
	if err != nil || status != 200 || !strings.Contains(body, `["Run the tests","Commit it"]`) {
		t.Fatalf("presets: %d %v %s", status, err, body)
	}
}

func TestPairThenListAndGuardedSend(t *testing.T) {
	r := newRig(t)
	device := r.pair(t)
	name := DeviceName(device.DeviceID)

	status, body, err := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", device.Token, "")
	if err != nil || status != 200 || !strings.Contains(body, `"demo"`) {
		t.Fatalf("list: %d %v %s", status, err, body)
	}

	r.states.Report("s1", agentstate.Working, agentstate.Cue, false)
	status, body, _ = r.call(t, name, device.Fingerprint, "POST", "/v1/sessions/s1/send", device.Token, `{"text":"secret prompt"}`)
	if status != 409 {
		t.Fatalf("a submit while working must be refused with 409, got %d %s", status, body)
	}
	status, _, _ = r.call(t, name, device.Fingerprint, "POST", "/v1/sessions/s1/send", device.Token, `{"text":"draft","insertOnly":true}`)
	if status != 200 || len(r.deliverd) != 1 {
		t.Fatalf("insert-only while working is allowed: %d, delivered %d", status, len(r.deliverd))
	}
	if status, _, _ = r.call(t, name, device.Fingerprint, "POST", "/v1/sessions/nope/send", device.Token, `{"text":"x"}`); status != 404 {
		t.Fatalf("unknown session: %d", status)
	}
	if status, _, _ = r.call(t, name, device.Fingerprint, "POST", "/v1/sessions/s1/start", device.Token, `{"mode":"start"}`); status != 409 {
		t.Fatalf("starting a running session must be refused: %d", status)
	}
	for _, line := range r.audit {
		if strings.Contains(line, "secret") || strings.Contains(line, "draft") {
			t.Fatalf("audit must never hold text: %q", line)
		}
	}
}

func TestAuthMatrixAndRevocation(t *testing.T) {
	r := newRig(t)
	device := r.pair(t)
	other := r.pair(t)
	name := DeviceName(device.DeviceID)

	if status, _, _ := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", "", ""); status != 401 {
		t.Fatalf("missing token: %d", status)
	}
	if status, _, _ := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", other.Token, ""); status != 401 {
		t.Fatalf("another device's token must not work: %d", status)
	}
	if _, _, err := r.call(t, name, other.Fingerprint, "GET", "/v1/sessions", device.Token, ""); err == nil {
		t.Fatal("pinning another device's certificate must fail the handshake")
	}
	if _, _, err := r.call(t, "d-unknown.taushell", device.Fingerprint, "GET", "/v1/sessions", device.Token, ""); err == nil {
		t.Fatal("an unknown device name must get no certificate")
	}
	if _, _, err := r.call(t, PairName, device.Fingerprint, "GET", "/v1/sessions", device.Token, ""); err == nil {
		t.Fatal("the pairing channel must be closed when no window is open")
	}
	if err := r.server.Registry.Revoke(device.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", device.Token, ""); err == nil {
		t.Fatal("a revoked device must fail the handshake")
	}
	if status, _, err := r.call(t, DeviceName(other.DeviceID), other.Fingerprint, "GET", "/v1/sessions", other.Token, ""); err != nil || status != 200 {
		t.Fatalf("revoking one device must not affect another: %d %v", status, err)
	}
}

func TestPairingRefusesWrongCodeAndOldTLS(t *testing.T) {
	r := newRig(t)
	credentials, _ := pairing.New([]string{"127.0.0.1"}, time.Minute)
	r.server.SetPairingCredentials(&credentials)
	r.server.Window.Open(time.Minute)
	if status, _, _ := r.call(t, PairName, credentials.Fingerprint, "POST", "/v1/pair", "", `{"code":"WRONGCODE1","name":"x"}`); status != 403 {
		t.Fatalf("wrong code: %d", status)
	}
	if status, _, _ := r.call(t, DeviceName("x"), credentials.Fingerprint, "POST", "/v1/pair", "", `{}`); status == 200 {
		t.Fatal("pairing must not work outside the pairing channel")
	}
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", r.address,
		&tls.Config{InsecureSkipVerify: true, ServerName: PairName, MaxVersion: tls.VersionTLS12})
	if err == nil {
		connection.Close()
		t.Fatal("TLS 1.2 must not be accepted")
	}
}

func TestFailedAuthBacksOff(t *testing.T) {
	r := newRig(t)
	device := r.pair(t)
	name := DeviceName(device.DeviceID)
	var last int
	for attempt := 0; attempt < maxFailures+2; attempt++ {
		last, _, _ = r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", "wrong", "")
	}
	if last != 429 {
		t.Fatalf("repeated failures must back off, got %d", last)
	}
	if status, _, _ := r.call(t, name, device.Fingerprint, "GET", "/v1/sessions", device.Token, ""); status != 429 {
		t.Fatalf("even the right token waits during back-off: %d", status)
	}
}
