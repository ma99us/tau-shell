package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validUser = `
schema: 1
id: demo-cli
name: Demo CLI
short: Demo
executable: demo
probes:
  version_args: ["--version"]
session:
  id: tau
  start_args: ["--session-id", "{id}"]
  resume_args: ["--resume", "{id}"]
install:
  vendor_url: https://example.com/demo
brand:
  mark: monogram
  color: "#4f8cff"
`

func TestBuiltinsLoadInDisplayOrder(t *testing.T) {
	var ids []string
	for _, item := range Builtins().All() {
		ids = append(ids, item.ID)
		if !item.Builtin {
			t.Errorf("%s is not marked built-in", item.ID)
		}
	}
	if got := strings.Join(ids, ","); got != "copilot-cli,claude-code,codex,opencode,gsd" {
		t.Fatalf("built-in order = %s", got)
	}
}

func TestDecodeRejectsInvalidUserManifests(t *testing.T) {
	replace := func(old, replacement string) string { return strings.Replace(validUser, old, replacement, 1) }
	for name, test := range map[string]struct{ text, want string }{
		"unknown field":       {validUser + "surprise: true\n", "surprise"},
		"wrong schema":        {replace("schema: 1", "schema: 2"), "schema"},
		"bad id":              {replace("id: demo-cli", "id: Demo CLI"), "id"},
		"path executable":     {replace("executable: demo", `executable: 'C:\tools\demo.exe'`), "executable"},
		"script executable":   {replace("executable: demo", "executable: demo.ps1"), "executable"},
		"relative executable": {replace("executable: demo", "executable: ./demo"), "executable"},
		"missing resume":      {replace(`  resume_args: ["--resume", "{id}"]`, ""), "session.resume_args"},
		"unknown placeholder": {replace("{id}\"]\ninstall", "{secret}\"]\ninstall"), "placeholder"},
		"unknown reader":      {validUser + "model: nope\n", "unknown reader"},
		"http vendor":         {replace("https://example.com/demo", "http://example.com/demo"), "https"},
		"winget for user":     {replace("  vendor_url: https://example.com/demo", "  vendor_url: https://example.com/demo\n  winget_id: A.B"), "built-in"},
		"vendor mark":         {replace("mark: monogram", "mark: claude"), "monogram"},
		"bad cue regex":       {validUser + "cues:\n  - {state: idle, source: title, pattern: '(?i)x'}\n", "inline flags"},
		"lookbehind":          {validUser + "cues:\n  - {state: idle, source: title, pattern: '(?<=a)b'}\n", "pattern"},
		"cue rows":            {validUser + "cues:\n  - {state: idle, source: last-rows, pattern: x}\n", "rows"},
		"discover no reader":  {replace("id: tau", "id: discover"), "session.discovery"},
		"id before launch":    {replace("id: tau", "id: discover\n  discovery: codex-rollout"), "{id}"},
		"two documents":       {validUser + "---\nid: other\n", "exactly one"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(test.text), false)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want it to mention %q", err, test.want)
			}
		})
	}
	if _, err := Decode([]byte(validUser), false); err != nil {
		t.Fatalf("valid user manifest rejected: %v", err)
	}
}

func TestLoadUserManifests(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("demo.yaml", validUser)
	write("zcopy.yml", validUser)
	write("claude.yaml", strings.Replace(validUser, "id: demo-cli", "id: claude-code", 1))
	write("broken.yaml", "schema: 1\nid: [")
	write("notes.txt", "ignored")

	registry := Load(dir)
	if _, ok := registry.Get("demo-cli"); !ok {
		t.Fatal("valid user manifest was not loaded")
	}
	if claude, _ := registry.Get("claude-code"); !claude.Builtin {
		t.Fatal("a user file replaced a built-in harness")
	}
	all := registry.All()
	if last := all[len(all)-1]; last.ID != "demo-cli" || last.Builtin || last.Hash == "" || last.Path == "" {
		t.Fatalf("user manifest should come last with a hash and path: %+v", last)
	}
	problems := map[string]string{}
	for _, problem := range registry.Errors() {
		problems[filepath.Base(problem.Path)] = problem.Error
	}
	if len(problems) != 3 || !strings.Contains(problems["claude.yaml"], "built-in") ||
		!strings.Contains(problems["zcopy.yml"], "already defined") || problems["broken.yaml"] == "" {
		t.Fatalf("problems = %v", problems)
	}
	if missing := Load(filepath.Join(dir, "absent")); len(missing.Errors()) != 0 || len(missing.All()) != 5 {
		t.Fatal("a missing manifest folder is normal")
	}
}

func TestHashChangesWithContent(t *testing.T) {
	first, err := Decode([]byte(validUser), false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Decode([]byte(validUser+"order: 3\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash == second.Hash {
		t.Fatal("hash must change when the file changes")
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("DEMO_HOME", "")
	got, err := ExpandPath("{env:DEMO_HOME|~/.demo}/sessions/{id}.jsonl", "abc")
	if err != nil || got != filepath.Join(home, ".demo", "sessions", "abc.jsonl") {
		t.Fatalf("fallback expansion = %q, %v", got, err)
	}
	t.Setenv("DEMO_HOME", t.TempDir())
	got, _ = ExpandPath("{env:DEMO_HOME|~/.demo}/x", "")
	if !strings.HasPrefix(got, os.Getenv("DEMO_HOME")) {
		t.Fatalf("env value ignored: %q", got)
	}
	for _, bad := range []string{"{secret}/x", "{id}/x"} {
		if _, err := ExpandPath(bad, ""); err == nil {
			t.Errorf("%q must not expand", bad)
		}
	}
}

func TestCompilePatternAcceptsUnicodeEscapes(t *testing.T) {
	pattern, err := CompilePattern(`^[\u2800-\u28FF]`, false)
	if err != nil || !pattern.MatchString("\u280b working") || pattern.MatchString("plain") {
		t.Fatalf("pattern = %v, %v", pattern, err)
	}
}
