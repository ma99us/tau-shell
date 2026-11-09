package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "init")
	return repo
}

func TestSlugAndValidate(t *testing.T) {
	if got := Slug("  Fix Login Bug!! "); got != "fix-login-bug" {
		t.Fatalf("slug = %q", got)
	}
	for _, bad := range []string{"", "A", "a..b", "nul", "x.lock", "-a", "a/b"} {
		if ValidateName(bad) == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if err := ValidateName("fix-login"); err != nil {
		t.Fatal(err)
	}
}

func TestParseList(t *testing.T) {
	out := "worktree /r/main\nHEAD abc\nbranch refs/heads/main\n\nworktree /r/main.worktrees/x\nHEAD def\nbranch refs/heads/taushell/x\n\nworktree /r/d\ndetached\nlocked\n"
	entries := parseList(context.Background(), out)
	if len(entries) != 3 || !entries[0].Main || entries[1].Branch != "taushell/x" || !entries[2].Detached || !entries[2].Locked {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestCreateInspectRemove(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	if _, ok := Inspect(ctx, repo); ok {
		t.Fatal("main checkout must not be a linked worktree")
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".worktreeinclude"), []byte(".env\na.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "more")

	result, err := Create(ctx, Spec{Dir: repo, Name: "feat", Branch: "taushell/feat"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(filepath.Dir(repo), "proj.worktrees", "feat"); !strings.EqualFold(result.Path, want) {
		t.Fatalf("path = %q, want %q", result.Path, want)
	}
	// Only ignored files are copied; tracked a.txt comes from git.
	if result.Copied != 1 {
		t.Fatalf("copied = %d, warnings %v", result.Copied, result.Warnings)
	}
	if data, _ := os.ReadFile(filepath.Join(result.Path, ".env")); string(data) != "SECRET=1" {
		t.Fatal(".env was not copied")
	}
	info, ok := Inspect(ctx, result.Path)
	if !ok || info.Name != "feat" || info.Branch != "taushell/feat" || !strings.EqualFold(info.MainRepo, repo) {
		t.Fatalf("info = %+v ok=%v", info, ok)
	}
	if _, err := Create(ctx, Spec{Dir: repo, Name: "feat", Branch: "taushell/other"}); err == nil {
		t.Fatal("existing folder must be rejected")
	}

	if err := os.WriteFile(filepath.Join(result.Path, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, result.Path, info.Branch, false, false); err == nil {
		t.Fatal("dirty worktree must be refused")
	}
	if err := Remove(ctx, repo, "", false, true); err == nil {
		t.Fatal("main checkout must never be removed")
	}
	if err := Remove(ctx, result.Path, info.Branch, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.Path); err == nil {
		t.Fatal("worktree folder still exists")
	}
	if list, _ := List(ctx, repo); len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
}

func TestCreateRejectsEmptyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	if _, err := Create(context.Background(), Spec{Dir: repo, Name: "x", Branch: "b"}); err == nil {
		t.Fatal("repo without commits must be rejected")
	}
}
