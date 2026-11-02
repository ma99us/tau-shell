// Package mobile is the phone-facing HTTPS API. It is a second front end for the
// same guarded session logic the local control channel uses (internal/control):
// every action passes the same refusal rules. Nothing here is wired into the
// app yet, and it carries no prompt text or terminal output in logs or audit.
package mobile

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/control"
	"github.com/ma99us/tau-shell/internal/pairing"
)

// SNI names select the certificate: "pair" for the one-time pairing channel and
// "d-<device id>" for an already paired phone, so every pairing has its own key.
const (
	PairName     = "pair"
	devicePrefix = "d-"
	maxBody      = 32 * 1024
	// maxFailures per minute from one address before it is refused for a while.
	maxFailures = 10
)

// DeviceName is the SNI name a phone uses for its device id.
func DeviceName(id string) string { return devicePrefix + id }

// Backend is the guarded session logic; control.Sessions satisfies it.
type Backend interface {
	HandleList() control.Response
	HandleWait(json.RawMessage) control.Response
	HandleSend(json.RawMessage) control.Response
	HandleStart(json.RawMessage) control.Response
}

// Server serves phones over TLS 1.3 with per-device certificates.
type Server struct {
	Registry *pairing.Registry
	Window   *pairing.Window
	Backend  Backend
	// Audit receives device id, operation, session id and outcome. Never text.
	Audit func(device, op, session, outcome string)
	// Clock and PairWait are overridable for tests.
	Clock    func() time.Time
	PairWait time.Duration

	mu       sync.Mutex
	pairing  *pairing.Credentials
	failures map[string][]time.Time
}

// SetPairingCredentials installs the certificate used on the pairing channel.
// Call it when a pairing window opens; clear it with nil when the window closes.
func (server *Server) SetPairingCredentials(credentials *pairing.Credentials) {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.pairing = credentials
}

func (server *Server) now() time.Time {
	if server.Clock != nil {
		return server.Clock()
	}
	return time.Now()
}

// TLSConfig returns the server configuration. An unknown, revoked or expired
// name gets no certificate, so the handshake fails before any HTTP.
func (server *Server) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := hello.ServerName
			if name == PairName {
				server.mu.Lock()
				credentials := server.pairing
				server.mu.Unlock()
				if credentials == nil {
					return nil, errors.New("pairing is closed")
				}
				return &credentials.Certificate, nil
			}
			if id, ok := strings.CutPrefix(name, devicePrefix); ok {
				return server.Registry.Certificate(id)
			}
			return nil, errors.New("unknown name")
		},
	}
}

// Serve accepts phone connections until the listener closes.
func (server *Server) Serve(listener net.Listener) error {
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      3 * time.Minute,
		MaxHeaderBytes:    8 << 10,
	}
	return httpServer.Serve(tls.NewListener(listener, server.TLSConfig()))
}

// Handler routes /v1 requests.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/pair", server.handlePair)
	mux.HandleFunc("GET /v1/sessions", server.device("list", server.handleList))
	mux.HandleFunc("GET /v1/sessions/{id}/wait", server.device("wait", server.handleWait))
	mux.HandleFunc("POST /v1/sessions/{id}/send", server.device("send", server.handleSend))
	mux.HandleFunc("POST /v1/sessions/{id}/start", server.device("start", server.handleStart))
	return mux
}

type deviceHandler func(writer http.ResponseWriter, request *http.Request, device pairing.Device)

// device authenticates the device named by the SNI with its bearer token.
func (server *Server) device(op string, next deviceHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		remote, _, _ := net.SplitHostPort(request.RemoteAddr)
		if server.blocked(remote) {
			http.Error(writer, "too many failures", http.StatusTooManyRequests)
			return
		}
		id, ok := "", false
		if request.TLS != nil {
			id, ok = strings.CutPrefix(request.TLS.ServerName, devicePrefix)
		}
		token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		device, err := server.Registry.Authenticate(id, token)
		if !ok || err != nil {
			server.fail(remote)
			server.audit(id, op, "", "unauthorized")
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, maxBody)
		next(writer, request, device)
	}
}

func (server *Server) audit(device, op, session, outcome string) {
	if server.Audit != nil {
		server.Audit(device, op, session, outcome)
	}
}

func (server *Server) blocked(remote string) bool {
	server.mu.Lock()
	defer server.mu.Unlock()
	return len(server.recentLocked(remote)) >= maxFailures
}

func (server *Server) fail(remote string) {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.failures == nil {
		server.failures = map[string][]time.Time{}
	}
	server.failures[remote] = append(server.recentLocked(remote), server.now())
}

func (server *Server) recentLocked(remote string) []time.Time {
	cutoff := server.now().Add(-time.Minute)
	var kept []time.Time
	for _, at := range server.failures[remote] {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	if len(kept) == 0 {
		delete(server.failures, remote)
	} else {
		server.failures[remote] = kept
	}
	return kept
}

func writeResponse(writer http.ResponseWriter, response control.Response) {
	status := http.StatusOK
	if !response.OK {
		status = http.StatusInternalServerError
		if response.Error != nil {
			switch response.Error.Code {
			case control.CodeBadRequest:
				status = http.StatusBadRequest
			case control.CodeNotFound:
				status = http.StatusNotFound
			case control.CodeRefused:
				status = http.StatusConflict
			case control.CodeDisabled:
				status = http.StatusServiceUnavailable
			}
		}
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response)
}

func outcome(response control.Response) string {
	if response.OK {
		return "ok"
	}
	if response.Error != nil {
		return response.Error.Code
	}
	return "error"
}

func (server *Server) handleList(writer http.ResponseWriter, _ *http.Request, device pairing.Device) {
	response := server.Backend.HandleList()
	server.audit(device.ID, "list", "", outcome(response))
	writeResponse(writer, response)
}

func (server *Server) handleWait(writer http.ResponseWriter, request *http.Request, device pairing.Device) {
	timeoutMS := 0
	if raw := request.URL.Query().Get("timeoutMs"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeResponse(writer, control.Failure(control.CodeBadRequest, "timeoutMs must be a number"))
			return
		}
		timeoutMS = value
	}
	session := request.PathValue("id")
	args, _ := json.Marshal(control.WaitArgs{Session: session, For: request.URL.Query().Get("for"), TimeoutMS: timeoutMS})
	response := server.Backend.HandleWait(args)
	server.audit(device.ID, "wait", session, outcome(response))
	writeResponse(writer, response)
}

func (server *Server) handleSend(writer http.ResponseWriter, request *http.Request, device pairing.Device) {
	session := request.PathValue("id")
	var body struct {
		Text       string `json:"text"`
		InsertOnly bool   `json:"insertOnly"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		server.audit(device.ID, "send", session, control.CodeBadRequest)
		writeResponse(writer, control.Failure(control.CodeBadRequest, "send needs a JSON body with text"))
		return
	}
	args, _ := json.Marshal(control.SendArgs{Session: session, Text: body.Text, InsertOnly: body.InsertOnly})
	response := server.Backend.HandleSend(args)
	server.audit(device.ID, "send", session, outcome(response))
	writeResponse(writer, response)
}

func (server *Server) handleStart(writer http.ResponseWriter, request *http.Request, device pairing.Device) {
	session := request.PathValue("id")
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		server.audit(device.ID, "start", session, control.CodeBadRequest)
		writeResponse(writer, control.Failure(control.CodeBadRequest, "start needs a JSON body with mode"))
		return
	}
	args, _ := json.Marshal(control.StartArgs{Session: session, Mode: body.Mode})
	response := server.Backend.HandleStart(args)
	server.audit(device.ID, "start", session, outcome(response))
	writeResponse(writer, response)
}

// PairResult is what a successfully approved phone receives. The phone then
// reconnects with the SNI name "d-<DeviceID>" and pins Fingerprint.
type PairResult struct {
	DeviceID    string `json:"deviceId"`
	Token       string `json:"token"`
	Fingerprint string `json:"fingerprint"`
	Expires     string `json:"expires"`
}

// handlePair runs only on the pairing channel (SNI "pair"). The phone holds the
// request open until the PC user approves or denies.
func (server *Server) handlePair(writer http.ResponseWriter, request *http.Request) {
	remote, _, _ := net.SplitHostPort(request.RemoteAddr)
	if request.TLS == nil || request.TLS.ServerName != PairName {
		http.Error(writer, "pairing uses the pairing channel", http.StatusBadRequest)
		return
	}
	if server.blocked(remote) {
		http.Error(writer, "too many failures", http.StatusTooManyRequests)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxBody)
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		http.Error(writer, "bad request", http.StatusBadRequest)
		return
	}
	pending, err := server.Window.Submit(body.Code, body.Name)
	if err != nil {
		server.fail(remote)
		server.audit("", "pair", "", "refused")
		status := http.StatusForbidden
		if errors.Is(err, pairing.ErrBusy) {
			status = http.StatusConflict
		}
		http.Error(writer, err.Error(), status)
		return
	}
	wait := server.PairWait
	if wait <= 0 {
		wait = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(request.Context(), wait)
	defer cancel()
	go func() { // expiry is lazy in the window, so tick it while the phone waits
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				server.Window.State()
			}
		}
	}()
	result, err := pending.Wait(ctx)
	if err != nil {
		server.Window.Close()
		server.audit("", "pair", "", "timeout")
		http.Error(writer, "timed out waiting for approval on the PC", http.StatusRequestTimeout)
		return
	}
	if result.Err != nil {
		server.audit("", "pair", "", "denied")
		http.Error(writer, result.Err.Error(), http.StatusForbidden)
		return
	}
	server.audit(result.Device.ID, "pair", "", "ok")
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(PairResult{
		DeviceID: result.Device.ID, Token: result.Token,
		Fingerprint: result.Device.Fingerprint, Expires: result.Device.Expires.UTC().Format(time.RFC3339),
	})
}
