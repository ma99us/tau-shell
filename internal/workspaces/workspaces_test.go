package workspaces

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveGroupsByRepositoryRootOrFolder(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "App")
	sub := filepath.Join(repo, "pkg", "deep")
	plain := filepath.Join(root, "notes")
	for _, dir := range []string{filepath.Join(repo, ".git"), sub, plain} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	top, nested := Resolve(repo), Resolve(sub)
	if top.Key != nested.Key || top.Name != "App" || !top.Repo {
		t.Fatalf("repo and subfolder should share a workspace: %+v %+v", top, nested)
	}
	if got := Resolve(strings.ToUpper(repo)); got.Key != top.Key && filepath.VolumeName(repo) != "" {
		t.Errorf("keys should ignore case on Windows: %+v", got)
	}
	folder := Resolve(plain)
	if folder.Repo || folder.Name != "notes" || folder.Key == top.Key {
		t.Fatalf("plain folder = %+v", folder)
	}
	if got := Resolve(filepath.Join(root, "gone")); got.Key != MissingKey {
		t.Fatalf("missing folder = %+v", got)
	}
}

func TestLinkedWorktreeJoinsMainWorkspace(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main")
	tree := filepath.Join(root, "tree")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(filepath.Join(main, ".git", "worktrees", "tree"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{tree, sub} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pointer := "gitdir: " + filepath.ToSlash(filepath.Join(main, ".git", "worktrees", "tree"))
	if err := os.WriteFile(filepath.Join(tree, ".git"), []byte(pointer), 0o644); err != nil {
		t.Fatal(err)
	}
	if Resolve(tree).Key != Resolve(main).Key {
		t.Fatal("a linked worktree should join its main checkout's workspace")
	}
	submodule := "gitdir: " + filepath.ToSlash(filepath.Join(main, ".git", "modules", "x"))
	if err := os.WriteFile(filepath.Join(sub, ".git"), []byte(submodule), 0o644); err != nil {
		t.Fatal(err)
	}
	if Resolve(sub).Key == Resolve(main).Key {
		t.Fatal("a submodule must stay its own workspace")
	}
}
