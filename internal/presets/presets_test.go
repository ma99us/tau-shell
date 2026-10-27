package presets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingInvalidAndValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	list, err := Load(path)
	if err != nil || list != nil {
		t.Fatalf("missing file: got %v, %v", list, err)
	}
	if err := os.WriteFile(path, []byte("presets: [ {name: a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("invalid YAML must be an error")
	}
	if err := os.WriteFile(path, []byte("presets:\n  - name: a\n    text: hi\n    bogus: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("unknown field must be an error")
	}
	if err := os.WriteFile(path, []byte("presets:\n  - name: a\n    text: hi\n  - name: A\n    text: dup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate names must be rejected, got %v", err)
	}
}

func TestExampleFileLoadsAndIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	if err := EnsureExample(path); err != nil {
		t.Fatal(err)
	}
	list, err := Load(path)
	if err != nil || len(list) == 0 {
		t.Fatalf("example must load: %v, %v", list, err)
	}
	if err := os.WriteFile(path, []byte("presets: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureExample(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "presets: []\n" {
		t.Fatal("EnsureExample overwrote an existing file")
	}
}

func TestMergeSessionOverridesGlobal(t *testing.T) {
	merged := Merge(
		[]Preset{{Name: "A", Text: "global a"}, {Name: "B", Text: "global b"}},
		[]Preset{{Name: "a", Text: "session a"}},
	)
	if len(merged) != 2 || merged[0].Text != "global b" || merged[1].Text != "session a" {
		t.Fatalf("unexpected merge: %+v", merged)
	}
}

func TestResolveExpandsFiltersAndReportsUnknown(t *testing.T) {
	no := false
	dir := t.TempDir()
	resolved := Resolve(context.Background(), []Preset{
		{Name: "one", Text: "{project}|{session}|{dir}|{branch}|{nope}", Submit: &no},
		{Name: "claude only", Text: "x", Tools: []string{"claude-code"}},
	}, Context{Tool: "copilot-cli", Session: "S", WorkingDir: dir})
	if len(resolved) != 1 {
		t.Fatalf("tool filter failed: %+v", resolved)
	}
	item := resolved[0]
	want := filepath.Base(dir) + "|S|" + dir + "||{nope}"
	if item.Text != want || item.Submit || len(item.Unknown) != 1 || item.Unknown[0] != "nope" {
		t.Fatalf("got %+v, want text %q", item, want)
	}
}
