package appconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppearanceDefaultsAndValidation(t *testing.T) {
	if got := (Appearance{}).Resolve(); got != (ResolvedAppearance{Theme: "dark", FontScale: 1, TerminalFontSize: 14}) {
		t.Fatalf("defaults: %+v", got)
	}
	dir := t.TempDir()
	for name, content := range map[string]string{
		"theme": "appearance:\n  theme: sepia\n",
		"low":   "appearance:\n  font_scale: 0.5\n",
		"high":  "appearance:\n  font_scale: 2\n",
		"term":  "appearance:\n  terminal_font_size: 5\n",
	} {
		path := filepath.Join(dir, name+".yaml")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func TestSetAppearanceEditsInPlace(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"missing":  "",
		"no block": "# mine\nversion: 1\n\nnotifications:\n  toast: false\n",
		"example":  exampleFile,
		"explicit": "version: 1\nappearance:\n  theme: dark # keep\n  font_scale: 1.2\nusage:\n  month_start_day: 3\n",
	}
	for name, content := range cases {
		path := filepath.Join(dir, name+".yaml")
		if content != "" {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for _, want := range []ResolvedAppearance{
			{Theme: "light", FontScale: 1.25, TerminalFontSize: 16},
			{Theme: "system", FontScale: 1, TerminalFontSize: 14},
			{Theme: "light", FontScale: 1.4, TerminalFontSize: 12},
		} {
			if err := SetAppearance(path, want); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			config, err := Load(path)
			if err != nil || config.Appearance.Resolve() != want {
				t.Fatalf("%s: got %+v err=%v", name, config.Appearance.Resolve(), err)
			}
		}
		saved, _ := os.ReadFile(path)
		if strings.Count(string(saved), "\n  theme: light") != 1 {
			t.Fatalf("%s: expected one theme line:\n%s", name, saved)
		}
	}
	kept, _ := os.ReadFile(filepath.Join(dir, "no block.yaml"))
	if !strings.Contains(string(kept), "# mine") || !strings.Contains(string(kept), "toast: false") {
		t.Fatalf("lost content:\n%s", kept)
	}
	untouched := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(untouched, []byte("appearance:\n  theme: sepia\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetAppearance(untouched, ResolvedAppearance{Theme: "dark", FontScale: 1, TerminalFontSize: 14}); err == nil {
		t.Fatal("expected an invalid file to be refused")
	}
	if err := SetAppearance(filepath.Join(dir, "x.yaml"), ResolvedAppearance{Theme: "dark", FontScale: 3, TerminalFontSize: 14}); err == nil {
		t.Fatal("expected an out-of-range scale to be refused")
	}
}
