package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFileCreatesAndReplacesWithoutLeftovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	for _, content := range []string{"first", "second"} {
		if err := ReplaceFile(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("content = %q, %v; want %q", got, err, content)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory has %d entries (%v), want only the target", len(entries), err)
	}
}

func TestReplaceFileFailsWhenDirectoryIsMissing(t *testing.T) {
	if err := ReplaceFile(filepath.Join(t.TempDir(), "missing", "x"), []byte("x")); err == nil {
		t.Fatal("expected an error for a missing parent directory")
	}
}
