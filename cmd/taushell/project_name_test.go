package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeduceSessionName(t *testing.T) {
	write := func(dir, file, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name string
		file string
		body string
		want string
	}{
		{"npm", "package.json", `{"name":"@scope/web-app"}`, "web-app"},
		{"go", "go.mod", "module github.com/acme/tool\n\ngo 1.26\n", "tool"},
		{"cargo", "Cargo.toml", "[workspace]\nname = \"no\"\n[package]\nname = \"crate-x\"\n", "crate-x"},
		{"python", "pyproject.toml", "[project]\nname = 'pylib'\n", "pylib"},
		{"plain", "", "", "plain"},
		{"badjson", "package.json", "{", "badjson"},
		{"emptyname", "package.json", `{"name":""}`, "emptyname"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), test.name)
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if test.file != "" {
				write(dir, test.file, test.body)
			}
			if got := deduceSessionName(dir); got != test.want {
				t.Errorf("deduceSessionName = %q, want %q", got, test.want)
			}
		})
	}
}
