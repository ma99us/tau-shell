package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetControlEnabledEditsInPlace(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"missing":  "",
		"no block": "# mine\r\nversion: 1\r\n\r\nnotifications:\r\n  toast: false\r\n",
		"example":  exampleFile,
		"explicit": "version: 1\ncontrol:\n  enabled: false # keep\n  agents: false\nusage:\n  month_start_day: 3\n",
	}
	for name, content := range cases {
		path := filepath.Join(dir, name+".yaml")
		if content != "" {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for _, want := range []bool{true, false, true} {
			if err := SetControlEnabled(path, want); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			config, err := Load(path)
			if err != nil || config.Control.ChannelEnabled() != want {
				t.Fatalf("%s: enabled=%v err=%v", name, config.Control.ChannelEnabled(), err)
			}
		}
		saved, _ := os.ReadFile(path)
		if strings.Count(strings.ReplaceAll(string(saved), "\r\n", "\n"), "\n  enabled: true") != 1 {
			t.Fatalf("%s: expected one enabled line:\n%s", name, saved)
		}
	}
	kept, _ := os.ReadFile(filepath.Join(dir, "no block.yaml"))
	if !strings.Contains(string(kept), "# mine") || !strings.Contains(string(kept), "toast: false") || strings.Contains(strings.ReplaceAll(string(kept), "\r\n", ""), "\n") {
		t.Fatalf("comments, settings and CRLF must survive:\n%q", kept)
	}
	explicit, _ := os.ReadFile(filepath.Join(dir, "explicit.yaml"))
	if !strings.Contains(string(explicit), "# keep") || !strings.Contains(string(explicit), "month_start_day: 3") {
		t.Fatalf("explicit: %q", explicit)
	}
}

func TestSetControlEnabledLeavesInvalidFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	bad := "version: 1\nbogus: true\n"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetControlEnabled(path, true); err == nil {
		t.Fatal("invalid file must be rejected")
	}
	if got, _ := os.ReadFile(path); string(got) != bad {
		t.Fatalf("file changed: %q", got)
	}
}
