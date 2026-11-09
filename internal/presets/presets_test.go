package presets

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestParseGitStatus(t *testing.T) {
	clean := ParseGitStatus("# branch.oid abc\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -1\n")
	if clean.Branch != "main" || clean.Dirty || clean.Ahead != 2 || clean.Behind != 1 {
		t.Fatalf("unexpected clean status: %+v", clean)
	}
	dirty := ParseGitStatus("# branch.head (detached)\r\n1 .M N... 100644 100644 100644 a b file.txt\r\n? new.txt\r\n")
	if dirty.Branch != "detached" || !dirty.Dirty || dirty.Ahead != -1 || dirty.Behind != -1 {
		t.Fatalf("unexpected dirty status: %+v", dirty)
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

func TestGitSyncCommitsAndPushes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	git(root, "init", "--bare", remote)
	git(root, "clone", remote, work)
	git(work, "config", "user.name", "t")
	git(work, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(work, "add", "-A")
	git(work, "commit", "-m", "init")
	git(work, "push", "-u", "origin", "HEAD")
	if err := os.WriteFile(filepath.Join(work, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	summary, err := GitSync(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "master: committed local changes, pushed 1 commit. Workspace clean." && summary != "main: committed local changes, pushed 1 commit. Workspace clean." {
		t.Fatalf("unexpected summary %q", summary)
	}
	again, err := GitSync(context.Background(), work)
	if err != nil || !strings.HasSuffix(again, ": already in sync, nothing to do. Workspace clean.") {
		t.Fatalf("second sync: %q, %v", again, err)
	}
	status, _ := GitStatus(context.Background(), work)
	if status.Dirty || status.Ahead != 0 || status.Behind != 0 {
		t.Fatalf("workspace not clean after sync: %+v", status)
	}
}

func TestGitFetchMakesIncomingCommitsVisible(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	remote, first, second := filepath.Join(root, "remote.git"), filepath.Join(root, "first"), filepath.Join(root, "second")
	git(root, "init", "--bare", remote)
	git(root, "clone", remote, first)
	if err := os.WriteFile(filepath.Join(first, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(first, "add", "-A")
	git(first, "commit", "-m", "init")
	git(first, "push", "-u", "origin", "HEAD")
	git(root, "clone", remote, second)
	if err := os.WriteFile(filepath.Join(second, "b.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(second, "add", "-A")
	git(second, "commit", "-m", "incoming")
	git(second, "push")
	if status, _ := GitStatus(context.Background(), first); status.Behind != 0 {
		t.Fatalf("behind before fetch = %d, want 0", status.Behind)
	}
	if !GitFetch(context.Background(), first) {
		t.Fatal("expected a fetch to run")
	}
	if status, _ := GitStatus(context.Background(), first); status.Behind != 1 {
		t.Fatalf("behind after fetch = %d, want 1", status.Behind)
	}
	if GitFetch(context.Background(), first) {
		t.Fatal("a second fetch inside the throttle window should be skipped")
	}
	if GitFetch(context.Background(), t.TempDir()) {
		t.Fatal("a non-repository must not fetch")
	}
}

func TestParseGitChanges(t *testing.T) {
	output := " M src/a.go\x00A  src/new.go\x00?? src/dir/u.txt\x00 D src/gone.go\x00R  src/b.go\x00src/old.go\x00UU src/c.go\x00MM src/both.go\x00 M other/x.go\x00"
	got := ParseGitChanges(output, "src/")
	want := []GitChange{
		{"a.go", "M", false}, {"new.go", "A", true}, {"dir/u.txt", "U", false}, {"gone.go", "D", false},
		{"b.go", "R", true}, {"c.go", "!", false}, {"both.go", "M", false}, {"other/x.go", "M", false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestGitChangesInSubdirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if output, err := exec.Command("git", append([]string{"-C", root, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	sub := filepath.Join(root, "app")
	if err := os.MkdirAll(filepath.Join(sub, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "tracked.txt"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("init")
	run("add", "-A")
	run("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(sub, "tracked.txt"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "pkg", "new.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), []byte("o\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes, ok := GitChanges(context.Background(), sub)
	if !ok {
		t.Fatal("expected a repository")
	}
	want := []GitChange{{"tracked.txt", "M", false}, {"pkg/new.txt", "U", false}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("got %+v, want %+v", changes, want)
	}
	if _, ok := GitChanges(context.Background(), t.TempDir()); ok {
		t.Fatal("a non-repository must not report changes")
	}
}

func TestGitSyncRejectsNonRepository(t *testing.T) {
	if _, err := GitSync(context.Background(), t.TempDir()); err == nil {
		t.Fatal("expected an error outside a repository")
	}
}

func TestGitCommandRunsInsideWSLDistro(t *testing.T) {
	ctx := WithWSL(context.Background(), "Ubuntu")
	command, err := gitCommand(ctx, "/home/me/proj", []string{"GIT_TERMINAL_PROMPT=0"}, "status", "--short")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"wsl.exe", "-d", "Ubuntu", "--exec", "env", "GIT_TERMINAL_PROMPT=0", "git", "-C", "/home/me/proj", "status", "--short"}
	if strings.Join(command.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %q, want %q", command.Args, want)
	}
	if _, err := gitCommand(ctx, `C:\work`, nil, "status"); err == nil {
		t.Fatal("a Windows path must be rejected for a WSL repository")
	}
	native, err := gitCommand(context.Background(), `C:\work`, nil, "status")
	if err != nil || native.Args[len(native.Args)-3] != "-C" {
		t.Fatalf("native args = %q, %v", native.Args, err)
	}
}