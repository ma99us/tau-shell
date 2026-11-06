package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/appconfig"
	"github.com/ma99us/tau-shell/internal/control"
	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/mobile"
	"github.com/ma99us/tau-shell/internal/pairing"
	"github.com/skip2/go-qrcode"
)

const (
	mobileAuditLimit  = 200
	pairingWindowTTL  = 5 * time.Minute
	pairingCertTTL    = 10 * time.Minute
	pairedDeviceTTL   = 365 * 24 * time.Hour
	mobileDevicesFile = "devices.json"
)

// mobileRuntime is the running phone server. It exists only while enabled.
type mobileRuntime struct {
	server   *mobile.Server
	listener net.Listener
	host     string
	port     int
}

// mobileState is guarded by its own mutex and lives on App.
type mobileState struct {
	mu       sync.Mutex
	runtime  *mobileRuntime
	registry *pairing.Registry
	audit    *control.AuditLog
}

func (application *App) mobileDir() string {
	return filepath.Join(filepath.Dir(application.configDirectory), "mobile")
}

// mobileRegistryLocked opens the device registry on first use. The private keys
// are sealed with the Windows user's DPAPI key.
func (application *App) mobileRegistryLocked() (*pairing.Registry, error) {
	state := &application.mobile
	if state.registry != nil {
		return state.registry, nil
	}
	if err := os.MkdirAll(application.mobileDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create the phone state folder: %w", err)
	}
	registry, err := pairing.OpenRegistry(filepath.Join(application.mobileDir(), mobileDevicesFile), pairing.DPAPISealer{}, nil)
	if err != nil {
		return nil, err
	}
	state.registry = registry
	return registry, nil
}

func (application *App) mobileAuditLocked() *control.AuditLog {
	state := &application.mobile
	if state.audit == nil {
		state.audit = control.NewAuditLog(mobileAuditLimit, nil)
	}
	return state.audit
}

// localAddress reports whether host is an address of this PC.
func localAddress(host string) bool {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		if ip, _, err := net.ParseCIDR(address.String()); err == nil && ip.Equal(net.ParseIP(host)) {
			return true
		}
	}
	return false
}

// startMobile opens the phone server when settings enable it. Off by default;
// a failure is reported to the UI, never swallowed.
func (application *App) startMobile() {
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil {
		diagnostics.Warn("settings invalid; phone server stays off", err)
		return
	}
	if !settings.Mobile.ServerEnabled() {
		return
	}
	if err := application.applyMobile(true); err != nil {
		application.emit("settings-error", err.Error())
	}
}

func (application *App) stopMobile() { _ = application.applyMobile(false) }

// SetMobileEnabled starts or stops the phone server now and stores the choice.
func (application *App) SetMobileEnabled(enabled bool) error {
	if err := application.applyMobile(enabled); err != nil {
		return err
	}
	if err := appconfig.SetMobileEnabled(application.settingsPath(), enabled); err != nil {
		diagnostics.Error("save phone setting failed", err)
		return fmt.Errorf("The phone server is %s now, but the setting could not be saved: %w", map[bool]string{true: "on", false: "off"}[enabled], err)
	}
	return nil
}

func (application *App) applyMobile(enabled bool) error {
	state := &application.mobile
	state.mu.Lock()
	defer state.mu.Unlock()
	if !enabled {
		if state.runtime != nil {
			state.runtime.server.Window.Close()
			state.runtime.server.SetPairingCredentials(nil)
			_ = state.runtime.listener.Close()
			state.runtime = nil
			diagnostics.Info("phone server stopped")
		}
		return nil
	}
	if state.runtime != nil {
		return nil
	}
	settings, err := appconfig.Load(application.settingsPath())
	if err != nil {
		return fmt.Errorf("Phone server not started: %w", err)
	}
	host := settings.Mobile.Interface
	if host == "" {
		return errors.New("Phone server not started: set mobile.interface in config.yaml to this PC's LAN address (it never listens on all interfaces)")
	}
	if !localAddress(host) {
		return fmt.Errorf("Phone server not started: %s is not an address of this PC", host)
	}
	registry, err := application.mobileRegistryLocked()
	if err != nil {
		diagnostics.Error("phone registry failed to open", err)
		return fmt.Errorf("Phone server not started: %w", err)
	}
	audit := application.mobileAuditLocked()
	port := settings.Mobile.ListenPort()
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		diagnostics.Error("phone server failed to listen", err)
		return fmt.Errorf("Phone server could not listen on %s:%d: %w", host, port, err)
	}
	server := &mobile.Server{
		Registry: registry,
		Window:   pairing.NewWindow(registry, []string{host}, pairedDeviceTTL, nil),
		Backend:  application.controlSessions(),
		Presets: func() []string {
			current, err := appconfig.Load(application.settingsPath())
			if err != nil {
				return nil
			}
			return current.Mobile.Presets
		},
		SessionPresets: application.GetPresets,
		Audit: func(device, op, session, outcome string) {
			audit.Add(control.AuditEntry{Time: time.Now(), Image: "phone " + device, Op: op, Session: session, Outcome: outcome})
		},
	}
	state.runtime = &mobileRuntime{server: server, listener: listener, host: host, port: port}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			diagnostics.Error("phone server stopped unexpectedly", err)
		}
	}()
	diagnostics.Info("phone server listening")
	return nil
}

// PairingOffer is what the UI turns into a QR code. Code and Fingerprint are
// secrets until the window closes: never log them.
type PairingOffer struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Fingerprint string `json:"fingerprint"`
	Code        string `json:"code"`
	ExpiresAt   string `json:"expiresAt"`
	// Link is what the QR encodes; QR is that code as a PNG data URL.
	Link string `json:"link"`
	QR   string `json:"qr"`
}

// pairingLink is the payload the Android app scans. Every field is needed once.
func pairingLink(host string, port int, fingerprint, code string) string {
	query := url.Values{"host": {host}, "port": {strconv.Itoa(port)}, "fp": {fingerprint}, "code": {code}}
	return "taushell://pair?" + query.Encode()
}

// StartPairing opens a single, short-lived pairing window.
func (application *App) StartPairing() (PairingOffer, error) {
	state := &application.mobile
	state.mu.Lock()
	defer state.mu.Unlock()
	runtime := state.runtime
	if runtime == nil {
		return PairingOffer{}, errors.New("Turn the phone server on first")
	}
	credentials, err := pairing.New([]string{runtime.host}, pairingCertTTL)
	if err != nil {
		return PairingOffer{}, err
	}
	code, err := runtime.server.Window.Open(pairingWindowTTL)
	if err != nil {
		return PairingOffer{}, err
	}
	runtime.server.SetPairingCredentials(&credentials)
	link := pairingLink(runtime.host, runtime.port, credentials.Fingerprint, code)
	png, err := qrcode.Encode(link, qrcode.Medium, 256)
	if err != nil {
		runtime.server.Window.Close()
		runtime.server.SetPairingCredentials(nil)
		return PairingOffer{}, fmt.Errorf("could not draw the pairing QR code: %w", err)
	}
	return PairingOffer{
		Host: runtime.host, Port: runtime.port, Fingerprint: credentials.Fingerprint, Code: code,
		ExpiresAt: time.Now().Add(pairingWindowTTL).UTC().Format(time.RFC3339),
		Link:      link, QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	}, nil
}

// CancelPairing closes the window and the pairing channel.
func (application *App) CancelPairing() {
	state := &application.mobile
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.runtime != nil {
		state.runtime.server.Window.Close()
		state.runtime.server.SetPairingCredentials(nil)
	}
}

// DecidePairing approves or denies the phone waiting on the PC.
func (application *App) DecidePairing(approve bool) error {
	state := &application.mobile
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.runtime == nil {
		return errors.New("The phone server is off")
	}
	err := state.runtime.server.Window.Decide(approve)
	state.runtime.server.SetPairingCredentials(nil)
	return err
}

// RevokePhone deletes a paired phone; it cannot connect again without re-pairing.
func (application *App) RevokePhone(id string) error {
	state := &application.mobile
	state.mu.Lock()
	defer state.mu.Unlock()
	registry, err := application.mobileRegistryLocked()
	if err != nil {
		return err
	}
	return registry.Revoke(id)
}

// MobileInterface is one local address the phone server could listen on.
type MobileInterface struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// GetMobileInterfaces lists this PC's up IPv4 addresses. Loopback is included
// for USB-forwarded phones; the server never listens on all interfaces.
func (application *App) GetMobileInterfaces() ([]MobileInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}
	items := []MobileInterface{}
	for _, adapter := range interfaces {
		if adapter.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := adapter.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			if ip, _, err := net.ParseCIDR(address.String()); err == nil && ip.To4() != nil {
				items = append(items, MobileInterface{Name: adapter.Name, Address: ip.String()})
			}
		}
	}
	return items, nil
}

// SetMobileInterface stores the listen address and, when the server is on,
// restarts it there. A phone paired to another address must pair again.
func (application *App) SetMobileInterface(address string) error {
	if !localAddress(address) {
		return fmt.Errorf("%s is not an address of this PC", address)
	}
	if err := appconfig.SetMobileInterface(application.settingsPath(), address); err != nil {
		return err
	}
	application.mobile.mu.Lock()
	running := application.mobile.runtime != nil
	application.mobile.mu.Unlock()
	if !running {
		return nil
	}
	if err := application.applyMobile(false); err != nil {
		return err
	}
	return application.applyMobile(true)
}

// MobileStatus is the phone section of the Home/Settings UI.
type MobileStatus struct {
	Enabled     bool                 `json:"enabled"`
	Interface   string               `json:"interface,omitempty"`
	Address     string               `json:"address,omitempty"`
	Pairing     string               `json:"pairing"`
	PendingName string               `json:"pendingName,omitempty"`
	Devices     []pairing.Device     `json:"devices"`
	Audit       []control.AuditEntry `json:"audit"`
}

func (application *App) GetMobileStatus() (MobileStatus, error) {
	state := &application.mobile
	state.mu.Lock()
	defer state.mu.Unlock()
	status := MobileStatus{Pairing: string(pairing.StateIdle), Devices: []pairing.Device{}, Audit: []control.AuditEntry{}}
	if settings, err := appconfig.Load(application.settingsPath()); err == nil {
		status.Interface = settings.Mobile.Interface
	}
	if registry, err := application.mobileRegistryLocked(); err == nil {
		if devices := registry.List(); devices != nil {
			status.Devices = devices
		}
	} else {
		return status, err
	}
	if entries := application.mobileAuditLocked().Entries(); entries != nil {
		status.Audit = entries
	}
	if runtime := state.runtime; runtime != nil {
		status.Enabled = true
		status.Address = net.JoinHostPort(runtime.host, strconv.Itoa(runtime.port))
		status.Pairing = string(runtime.server.Window.State())
		if status.Pairing == string(pairing.StateIdle) {
			runtime.server.SetPairingCredentials(nil) // the window expired or was used up
		}
		if name, ok := runtime.server.Window.PendingName(); ok {
			status.PendingName = name
		}
	}
	return status, nil
}
