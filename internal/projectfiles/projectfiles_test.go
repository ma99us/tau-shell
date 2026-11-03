package projectfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListHidesNoiseSortsFoldersFirstAndStaysInside(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "b.md"), "b")
	write(t, filepath.Join(root, "docs", "plan.md"), "# Plan")
	write(t, filepath.Join(root, ".git", "config"), "x")
	write(t, filepath.Join(root, "node_modules", "x.js"), "x")
	listing, err := List(root, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range listing.Entries {
		names = append(names, entry.Path)
	}
	if strings.Join(names, ",") != "docs,b.md" {
		t.Fatalf("folders first, noise hidden: %v", names)
	}
	all, _ := List(root, "", true)
	if len(all.Entries) != 4 {
		t.Fatalf("showAll lists noise too: %+v", all.Entries)
	}
	sub, err := List(root, "docs", false)
	if err != nil || len(sub.Entries) != 1 || sub.Entries[0].Path != "docs/plan.md" {
		t.Fatalf("relative paths use forward slashes: %+v %v", sub, err)
	}
}

func TestPathsOutsideTheRootAreRefused(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	write(t, filepath.Join(root, "ok.txt"), "ok")
	write(t, filepath.Join(parent, "secret.txt"), "secret")
	for _, rel := range []string{"..", "../secret.txt", "..\\secret.txt", "docs/../../secret.txt", filepath.Join(parent, "secret.txt")} {
		if _, err := Read(root, rel); err == nil {
			t.Fatalf("%q must be refused", rel)
		}
		if _, err := List(root, rel, false); err == nil {
			t.Fatalf("listing %q must be refused", rel)
		}
	}
	if file, err := Read(root, "ok.txt"); err != nil || file.Text != "ok" {
		t.Fatalf("inside is fine: %+v %v", file, err)
	}
}

func TestSymlinksLeavingTheRootAreRefusedAndOmitted(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	write(t, filepath.Join(root, "ok.txt"), "ok")
	write(t, filepath.Join(parent, "outside", "secret.txt"), "secret")
	if err := os.Symlink(filepath.Join(parent, "outside"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}
	if _, err := Read(root, "link/secret.txt"); err == nil {
		t.Fatal("a link out of the root must be refused")
	}
	listing, _ := List(root, "", false)
	for _, entry := range listing.Entries {
		if entry.Name == "link" {
			t.Fatal("a link out of the root must not be listed")
		}
	}
}

func TestReadBoundsAndRefusesBinary(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "big.txt"), strings.Repeat("é", MaxReadBytes))
	file, err := Read(root, "big.txt")
	if err != nil || !file.Truncated || len(file.Text) > MaxReadBytes {
		t.Fatalf("large files are cut on a character boundary: %v truncated=%v len=%d", err, file.Truncated, len(file.Text))
	}
	write(t, filepath.Join(root, "bin.dat"), "a\x00b")
	if _, err := Read(root, "bin.dat"); err == nil {
		t.Fatal("binary files are refused")
	}
	if _, err := Read(root, "."); err == nil {
		t.Fatal("a folder is not a file")
	}
}
