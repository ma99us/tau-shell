package harness

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/session"
	"github.com/ma99us/tau-shell/internal/tools"
)

const userHarnessYAML = `
schema: 1
id: demo-cli
name: Demo CLI
short: Demo
executable: demo-cli
probes:
  version_args: [--version]
  help_args: [--help]
capabilities: []
launch:
  default_args: [--fast]
  reserved_args: [--session-id, --resume]
session:
  id: tau
  start_args: [--session-id, "{id}"]
  resume_args: [--resume, "{id}"]
graceful_exit:
  - input: "/exit\r"
    pause_ms: 100
install:
  vendor_url: https://example.com/demo
brand:
  mark: monogram
  text: Dm
  color: "#123456"
`

// A harness defined only by a user manifest must work through the same
// adapter, spec, launch-argument and trust paths as the built-in ones.
func TestUserManifestHarnessWorksEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.yaml"), []byte(userHarnessYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "demo-cli.exe"), []byte("MZ"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	registry := manifest.Load(dir)
	if errs := registry.Errors(); len(errs) != 0 {
		t.Fatalf("load errors: %v", errs)
	}
	manifest.SetDefault(registry)
	manifest.SetDefaultTrust(manifest.NewTrust(filepath.Join(dir, "trust.json")))
	t.Cleanup(func() { manifest.SetDefault(nil); manifest.SetDefaultTrust(nil) })

	id := session.Harness("demo-cli")
	if !id.Supported() {
		t.Fatal("user harness should be supported")
	}
	adapter, err := For(id)
	if err != nil {
		t.Fatal(err)
	}
	if path, err := adapter.FindExecutable(); err != nil || filepath.Base(path) != "demo-cli.exe" {
		t.Fatalf("executable = %q, %v", path, err)
	}
	if got, want := adapter.Argv([]string{"--fast"}, false, "abc"), []string{"--fast", "--session-id", "abc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("start argv = %v, want %v", got, want)
	}
	if got, want := adapter.Argv([]string{"--fast"}, true, "abc"), []string{"--fast", "--resume", "abc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("resume argv = %v, want %v", got, want)
	}
	if err := adapter.CheckResume("abc"); err != nil {
		t.Errorf("a harness without a resume reader must not block resume: %v", err)
	}
	if steps := adapter.GracefulExit(); len(steps) != 1 || steps[0].Input != "/exit\r" {
		t.Errorf("graceful exit = %+v", steps)
	}
	if err := session.ValidateLaunchArgs(id, []string{"--resume", "x"}); err == nil {
		t.Error("reserved launch args must be rejected for user harnesses")
	}
	spec, ok := tools.Lookup(tools.ID("demo-cli"))
	if !ok || spec.Install.VendorURL != "https://example.com/demo" {
		t.Errorf("tool spec = %+v, %v", spec, ok)
	}
	if err := manifest.Allowed("demo-cli"); err == nil {
		t.Error("an unconfirmed user harness must not launch")
	}
	item, _ := manifest.Get("demo-cli")
	if err := manifest.DefaultTrust().Approve(item); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Allowed("demo-cli"); err != nil {
		t.Errorf("confirmed harness refused: %v", err)
	}
}
