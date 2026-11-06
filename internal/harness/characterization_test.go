package harness

import (
	"reflect"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/harness/manifest"
	"github.com/ma99us/tau-shell/internal/session"
)

// These tests freeze the per-harness behaviour that the declarative manifests
// must reproduce exactly.
func TestHarnessCharacterization(t *testing.T) {
	ms := time.Millisecond
	gsdPath := GSDSessionPath("abc")
	type want struct {
		executable string
		defaults   []string
		start      []string
		resume     []string
		discovers  bool
		skillDir   bool
		exit       []ExitStep
	}
	cases := map[session.Harness]want{
		session.ClaudeCode: {"claude", []string{"--dangerously-skip-permissions"}, []string{"--session-id", "abc"}, []string{"--resume", "abc"}, false, true,
			[]ExitStep{{"\x03", 400 * ms}, {"/exit", 400 * ms}, {"\r", 0}}},
		session.CopilotCLI: {"copilot", []string{"--yolo"}, []string{"--session-id", "abc"}, []string{"--resume", "abc"}, false, true,
			[]ExitStep{{"\x03", 400 * ms}, {"/exit", 200 * ms}, {"\r", 0}}},
		session.GSD: {"gsd", nil, []string{"--session", gsdPath}, []string{"--session", gsdPath}, false, false,
			[]ExitStep{{"\x1b", 400 * ms}, {"/quit", 300 * ms}, {"\r", 0}}},
		session.Codex: {"codex", []string{"--dangerously-bypass-approvals-and-sandbox"}, nil, []string{"resume", "abc"}, true, true,
			[]ExitStep{{"\x1b", 400 * ms}, {"/exit", 300 * ms}, {"\r", 0}}},
		session.OpenCode: {"opencode", []string{"--auto"}, nil, []string{"--session", "abc"}, true, false,
			[]ExitStep{{"\x1b", 300 * ms}, {"\x1b", 400 * ms}, {"/exit", 300 * ms}, {"\r", 0}}},
	}
	for id, w := range cases {
		adapter, err := For(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := adapter.ID(); got != id {
			t.Errorf("%s ID = %s", id, got)
		}
		if got := adapter.StartArgs("abc"); !reflect.DeepEqual(got, w.start) {
			t.Errorf("%s start = %#v, want %#v", id, got, w.start)
		}
		if got := adapter.ResumeArgs("abc"); !reflect.DeepEqual(got, w.resume) {
			t.Errorf("%s resume = %#v, want %#v", id, got, w.resume)
		}
		if got := adapter.DefaultLaunchArgs(); !reflect.DeepEqual(got, w.defaults) && (len(got) != 0 || len(w.defaults) != 0) {
			t.Errorf("%s defaults = %#v, want %#v", id, got, w.defaults)
		}
		if got := adapter.DiscoversSessionID(); got != w.discovers {
			t.Errorf("%s discovers = %t", id, got)
		}
		if got := adapter.HasSkillDirectoryRoute(); got != w.skillDir {
			t.Errorf("%s skill dir = %t", id, got)
		}
		if id == session.Codex {
			item, ok := manifest.Get(string(id))
			if !ok {
				t.Error("Codex manifest is missing")
			} else if item.Skills.SkillDirOption() != "--add-dir" {
				t.Errorf("Codex skill directory option = %q, want --add-dir", item.Skills.SkillDirOption())
			}
		}
		if got := adapter.GracefulExit(); !reflect.DeepEqual(got, w.exit) {
			t.Errorf("%s exit = %#v, want %#v", id, got, w.exit)
		}
		path, err := adapter.FindExecutable()
		_ = path
		if err != nil && !containsAll(err.Error(), w.executable) {
			t.Errorf("%s lookup error should name %s: %v", id, w.executable, err)
		}
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		found := false
		for i := 0; i+len(p) <= len(s); i++ {
			if s[i:i+len(p)] == p {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestUnknownHarnessHasNoAdapter(t *testing.T) {
	if _, err := For("nope"); err == nil {
		t.Fatal("unknown harness must be rejected")
	}
}
