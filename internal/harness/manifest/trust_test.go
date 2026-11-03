package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The frontend cue engine reads the same JSON the host serves. This fixture is
// what its tests load, so a cue edit in a built-in manifest must be mirrored
// here. Set UPDATE_GOLDEN=1 to regenerate it.
func TestFrontendCueFixtureMatchesBuiltins(t *testing.T) {
	data, err := json.MarshalIndent(Builtins().All(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	path := filepath.Join("..", "..", "..", "frontend", "test", "fixtures", "harnesses.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture (run with UPDATE_GOLDEN=1): %v", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), data) {
		t.Fatal("frontend/test/fixtures/harnesses.json is out of date; run UPDATE_GOLDEN=1 go test ./internal/harness/manifest")
	}
}

func TestTrustIsBoundToTheManifestHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.json")
	trust, err := OpenTrust(path)
	if err != nil {
		t.Fatal(err)
	}
	user := &Manifest{ID: "mine", Hash: "aaa"}
	if trust.Trusted(user) {
		t.Fatal("an unconfirmed user manifest must not be trusted")
	}
	if !trust.Trusted(&Manifest{ID: "claude-code", Builtin: true}) {
		t.Fatal("built-ins are always trusted")
	}
	if err := trust.Approve(user); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenTrust(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Trusted(user) {
		t.Fatal("the confirmation must survive a restart")
	}
	if reopened.Trusted(&Manifest{ID: "mine", Hash: "bbb"}) {
		t.Fatal("an edited manifest must be confirmed again")
	}
	if err := trust.Approve(&Manifest{ID: "claude-code", Builtin: true}); err == nil {
		t.Fatal("built-ins need no confirmation")
	}
}

func TestAllowedRefusesUnconfirmedUserHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mine.yaml"), []byte(strings.Replace(validUser, "id: demo-cli", "id: mine", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := Load(dir)
	if len(registry.Errors()) != 0 {
		t.Fatalf("load errors: %v", registry.Errors())
	}
	SetDefault(registry)
	SetDefaultTrust(NewTrust(filepath.Join(dir, "trust.json")))
	t.Cleanup(func() { SetDefault(nil); SetDefaultTrust(nil) })
	if err := Allowed("mine"); err == nil {
		t.Fatal("unconfirmed user harness must be refused")
	}
	item, _ := registry.Get("mine")
	if err := DefaultTrust().Approve(item); err != nil {
		t.Fatal(err)
	}
	if err := Allowed("mine"); err != nil {
		t.Fatal(err)
	}
	if err := Allowed("claude-code"); err != nil {
		t.Fatal(err)
	}
}
