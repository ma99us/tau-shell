package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileDefaultsOffAndValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	config, err := Load(path)
	if err != nil || config.Mobile.ServerEnabled() || config.Mobile.ListenPort() != DefaultMobilePort {
		t.Fatalf("mobile must default off: %+v %v", config.Mobile, err)
	}
	for name, text := range map[string]string{
		"all interfaces": "version: 1\nmobile:\n  interface: 0.0.0.0\n",
		"not an address": "version: 1\nmobile:\n  interface: wifi\n",
		"low port":       "version: 1\nmobile:\n  port: 80\n",
	} {
		_ = os.WriteFile(path, []byte(text), 0o600)
		if _, err := Load(path); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
	_ = os.WriteFile(path, []byte("version: 1\nmobile:\n  enabled: true\n  interface: 192.168.1.20\n  port: 5000\n"), 0o600)
	if config, err = Load(path); err != nil || !config.Mobile.ServerEnabled() || config.Mobile.ListenPort() != 5000 {
		t.Fatalf("valid mobile settings: %+v %v", config.Mobile, err)
	}
}

func TestSetMobileEnabledKeepsControlAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	_ = os.WriteFile(path, []byte("version: 1\ncontrol:\n  enabled: true # keep\nmobile:\n  interface: 10.0.0.5\n"), 0o600)
	if err := SetMobileEnabled(path, true); err != nil {
		t.Fatal(err)
	}
	config, _ := Load(path)
	raw, _ := os.ReadFile(path)
	if !config.Mobile.ServerEnabled() || !config.Control.ChannelEnabled() || config.Mobile.Interface != "10.0.0.5" || !strings.Contains(string(raw), "# keep") {
		t.Fatalf("edit must be in place: %s", raw)
	}
	if err := SetMobileEnabled(path, false); err != nil {
		t.Fatal(err)
	}
	if config, _ = Load(path); config.Mobile.ServerEnabled() || !config.Control.ChannelEnabled() {
		t.Fatal("turning mobile off must not touch control")
	}
}
