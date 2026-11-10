//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddToGitignoreUsesTheNearestExistingFile(t *testing.T) {
	top := t.TempDir()
	sub := filepath.Join(top, "app", "build")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(top, ".gitignore"), []byte("node_modules/"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "out.log")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := addToGitignore(top, "", file, "app/build/out.log"); err != nil {
		t.Fatal(err)
	}
	if _, err := addToGitignore(top, "", sub, "app/build"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(top, ".gitignore"))
	if got, want := string(data), "node_modules/\n/app/build/out.log\n/app/build/\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	summary, err := addToGitignore(top, "", file, "app/build/out.log")
	if err != nil || !strings.Contains(summary, "already") {
		t.Fatalf("duplicate: %q, %v", summary, err)
	}

	nested := filepath.Join(top, "app", ".gitignore")
	if err := os.WriteFile(nested, []byte("*.tmp\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := addToGitignore(top, "", file, "app/build/out.log"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(nested)
	if got, want := string(data), "*.tmp\r\n/build/out.log\r\n"; got != want {
		t.Fatalf("nested got %q, want %q", got, want)
	}
}

func TestAddToGitignoreCreatesOneAtTheRepositoryRoot(t *testing.T) {
	top := t.TempDir()
	work := filepath.Join(top, "pkg")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(work, "a b#.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// The working directory is pkg, one level below the repository root.
	if _, err := addToGitignore(work, "pkg", file, "a b#.txt"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(top, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "/pkg/a b#.txt\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
