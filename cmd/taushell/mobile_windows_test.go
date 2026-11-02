package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/mobile"
	"github.com/ma99us/tau-shell/internal/pairing"
)

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func mobileApp(t *testing.T, settings string) *App {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	return &App{configDirectory: filepath.Join(root, "sessions")}
}

func TestPhoneServerNeedsAnExplicitLocalInterface(t *testing.T) {
	for name, settings := range map[string]string{
		"no interface":  "version: 1\n",
		"not this PC":   "version: 1\nmobile:\n  interface: 203.0.113.9\n",
		"all addresses": "version: 1\nmobile:\n  interface: 0.0.0.0\n",
	} {
		application := mobileApp(t, settings)
		if err := application.SetMobileEnabled(true); err == nil {
			t.Fatalf("%s: the server must refuse to start", name)
		}
		if status, _ := application.GetMobileStatus(); status.Enabled {
			t.Fatalf("%s: the server must stay off", name)
		}
	}
}

func TestPhoneServerPairsRevokesAndStops(t *testing.T) {
	port := freePort(t)
	application := mobileApp(t, "version: 1\nmobile:\n  interface: 127.0.0.1\n  port: "+strconv.Itoa(port)+"\n")
	if err := application.SetMobileEnabled(true); err != nil {
		t.Fatal(err)
	}
	defer application.stopMobile()
	raw, _ := os.ReadFile(application.settingsPath())
	if !strings.Contains(string(raw), "enabled: true") {
		t.Fatalf("the switch must be persisted: %s", raw)
	}

	offer, err := application.StartPairing()
	if err != nil || offer.Port != port || offer.Code == "" || offer.Fingerprint == "" {
		t.Fatalf("offer: %+v %v", offer, err)
	}
	config := pairing.PinnedClientConfig(offer.Fingerprint)
	config.ServerName = mobile.PairName
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: config, DisableKeepAlives: true}, Timeout: 10 * time.Second}
	type reply struct {
		status int
		result mobile.PairResult
		err    error
	}
	done := make(chan reply, 1)
	go func() {
		response, err := client.Post("https://127.0.0.1:"+strconv.Itoa(port)+"/v1/pair", "application/json",
			strings.NewReader(`{"code":"`+offer.Code+`","name":"Pixel"}`))
		if err != nil {
			done <- reply{err: err}
			return
		}
		defer response.Body.Close()
		var result mobile.PairResult
		_ = json.NewDecoder(response.Body).Decode(&result)
		done <- reply{status: response.StatusCode, result: result}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, _ := application.GetMobileStatus()
		if status.PendingName == "Pixel" && status.Pairing == string(pairing.StateAwaiting) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the PC never showed the waiting phone: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := application.DecidePairing(true); err != nil {
		t.Fatal(err)
	}
	paired := <-done
	if paired.err != nil || paired.status != 200 || paired.result.Token == "" {
		t.Fatalf("pairing: %+v", paired)
	}
	status, _ := application.GetMobileStatus()
	if len(status.Devices) != 1 || status.Pairing != string(pairing.StateIdle) || !status.Enabled {
		t.Fatalf("status after pairing: %+v", status)
	}
	if err := application.RevokePhone(paired.result.DeviceID); err != nil {
		t.Fatal(err)
	}
	if status, _ = application.GetMobileStatus(); len(status.Devices) != 0 {
		t.Fatal("a revoked phone must be gone")
	}

	if err := application.SetMobileEnabled(false); err != nil {
		t.Fatal(err)
	}
	if connection, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second); err == nil {
		connection.Close()
		t.Fatal("the port must be closed after the switch is turned off")
	}
}
