package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func skillFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, content := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(content)}
	}
	return fsys
}

const goodSkill = "---\nname: alpha\ndescription: Does alpha things\n---\nBody\n"

func TestBundledSnapshotIsValid(t *testing.T) {
	loaded := Load(Bundled(), SourceBundled, "bundled")
	if len(loaded.Warnings) > 0 {
		t.Fatalf("bundled warnings: %v", loaded.Warnings)
	}
	manifest, ok, err := ParseManifest(Bundled())
	if err != nil || !ok {
		t.Fatalf("bundled manifest: ok=%v err=%v", ok, err)
	}
	if len(loaded.Skills) != len(manifest.Skills) || len(loaded.Skills) == 0 {
		t.Fatalf("loaded %d skills, manifest lists %d", len(loaded.Skills), len(manifest.Skills))
	}
}

func TestLoadSkipsInvalidSkillsWithWarning(t *testing.T) {
	loaded := Load(skillFS(map[string]string{
		"skills/alpha/SKILL.md": goodSkill,
		"skills/beta/SKILL.md":  "---\nname: other\ndescription: x\n---\n",
		"skills/gamma/SKILL.md": "no frontmatter",
		"skills/delta/readme":   "no skill file",
	}), SourceLocal, "test")
	if len(loaded.Skills) != 1 || loaded.Skills[0].Name != "alpha" {
		t.Fatalf("skills = %+v", loaded.Skills)
	}
	if len(loaded.Warnings) != 3 {
		t.Fatalf("warnings = %v", loaded.Warnings)
	}
}

func TestManifestLimitsSkillsAndGatesPresets(t *testing.T) {
	loaded := Load(skillFS(map[string]string{
		ManifestName:            "version: 1\nskills: [alpha]\npresets:\n  - {name: Run, text: go, skill: alpha}\n",
		"skills/alpha/SKILL.md": goodSkill,
		"skills/beta/SKILL.md":  "---\nname: beta\ndescription: b\n---\n",
	}), SourceLocal, "test")
	if len(loaded.Skills) != 1 || len(loaded.Presets) != 1 || loaded.Presets[0].Skill != "alpha" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestMergePrecedenceAndMaterialize(t *testing.T) {
	high := Load(skillFS(map[string]string{"skills/alpha/SKILL.md": goodSkill, "skills/alpha/high.txt": "h"}), SourceLocal, "high")
	low := Load(skillFS(map[string]string{"skills/alpha/SKILL.md": goodSkill, "skills/alpha/low.txt": "l"}), SourceBundled, "low")
	merged := Merge([]Loaded{high, low})
	if len(merged) != 1 || merged[0].Source != SourceLocal {
		t.Fatalf("merged = %+v", merged)
	}
	root := t.TempDir()
	first, _, err := Materialize(root, merged)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := Materialize(root, merged)
	if err != nil || first != second {
		t.Fatalf("not idempotent: %q %q %v", first, second, err)
	}
	for _, name := range []string{".claude-plugin/plugin.json", "plugin.json", "skills/alpha/SKILL.md", "skills/alpha/high.txt"} {
		if _, err := os.Stat(filepath.Join(first, filepath.FromSlash(name))); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	changed, _, err := Materialize(root, Merge([]Loaded{low}))
	if err != nil || changed == first {
		t.Fatalf("different content must use a new folder: %q %v", changed, err)
	}
}

func TestLibraryPrepareSkipsPersonalAndDisabled(t *testing.T) {
	home := t.TempDir()
	personal := filepath.Join(home, "personal")
	if err := os.MkdirAll(filepath.Join(personal, "pr-to-main"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personal, "pr-to-main", "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.Repo = ""
	library, err := NewLibrary(filepath.Join(home, "tau"), config, map[string][]string{"claude-code": {personal}})
	if err != nil {
		t.Fatal(err)
	}
	if err := library.SetEnabled("windows-screenshot", false); err != nil {
		t.Fatal(err)
	}
	dir, names, _, err := library.Prepare("claude-code")
	if err != nil || dir == "" {
		t.Fatalf("prepare: %q %v", dir, err)
	}
	for _, name := range names {
		if name == "pr-to-main" || name == "windows-screenshot" {
			t.Fatalf("%s must be skipped, got %v", name, names)
		}
	}
	if !library.Available("claude-code")["pr-to-main"] {
		t.Fatal("personal skill must count as available")
	}
	reopened, _ := NewLibrary(filepath.Join(home, "tau"), config, nil)
	for _, entry := range reopened.List("claude-code") {
		if entry.Name == "windows-screenshot" && entry.Enabled {
			t.Fatal("disabled state must persist")
		}
	}
}

func TestParseRepoAndRef(t *testing.T) {
	if _, _, err := ParseRepo("https://github.com/mgerdov-igt/ai-cookbook"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"http://github.com/a/b", "https://evil.com/a/b", "https://github.com/a", "https://github.com/a/b/c", "git@github.com:a/b"} {
		if _, _, err := ParseRepo(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
	if ValidRef("../x") || ValidRef("a b") || !ValidRef("feature/x-1") {
		t.Fatal("ref validation wrong")
	}
}

func tarball(t *testing.T, entries []tar.Header, bodies []string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(gz)
	for index, header := range entries {
		if header.Typeflag == tar.TypeReg {
			header.Size = int64(len(bodies[index]))
		}
		if err := archive.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if _, err := archive.Write([]byte(bodies[index])); err != nil {
			t.Fatal(err)
		}
	}
	archive.Close()
	gz.Close()
	return buffer.Bytes()
}

func fakeGitHub(t *testing.T, sha string, archive []byte) (*Remote, *int) {
	t.Helper()
	calls := new(int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/o/r/commits/"):
			if r.Header.Get("If-None-Match") == `"e1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"e1"`)
			_, _ = w.Write([]byte(sha))
		case strings.HasPrefix(r.URL.Path, "/o/r/tar.gz/"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return &Remote{Owner: "o", Repo: "r", Ref: "main", API: server.URL, Codeload: server.URL, Client: server.Client()}, calls
}

const testSHA = "0123456789abcdef0123456789abcdef01234567"

func TestRemoteLatestUsesETag(t *testing.T) {
	remote, _ := fakeGitHub(t, testSHA, nil)
	sha, etag, changed, err := remote.Latest(context.Background(), "")
	if err != nil || !changed || sha != testSHA || etag != `"e1"` {
		t.Fatalf("first: %q %q %v %v", sha, etag, changed, err)
	}
	_, _, changed, err = remote.Latest(context.Background(), etag)
	if err != nil || changed {
		t.Fatalf("second must be not modified: %v %v", changed, err)
	}
}

func TestRemoteFetchExtractsOnlyWantedFiles(t *testing.T) {
	archive := tarball(t, []tar.Header{
		{Name: "r-sha/tau-shell.yaml", Typeflag: tar.TypeReg},
		{Name: "r-sha/skills/alpha/SKILL.md", Typeflag: tar.TypeReg},
		{Name: "r-sha/docs/big.md", Typeflag: tar.TypeReg},
	}, []string{"version: 1\n", goodSkill, "ignored"})
	remote, _ := fakeGitHub(t, testSHA, archive)
	dest := filepath.Join(t.TempDir(), "o-r", testSHA)
	if err := remote.Fetch(context.Background(), testSHA, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "skills", "alpha", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "docs")); err == nil {
		t.Fatal("files outside skills/ must not be extracted")
	}
}

func TestRemoteFetchRejectsUnsafeArchives(t *testing.T) {
	cases := map[string][]tar.Header{
		"symlink":   {{Name: "r-sha/skills/a/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}},
		"backslash": {{Name: `r-sha/skills/..\evil`, Typeflag: tar.TypeReg}},
		"too large": {{Name: "r-sha/skills/a/big", Typeflag: tar.TypeReg}},
	}
	for name, headers := range cases {
		body := "x"
		if name == "too large" {
			body = strings.Repeat("x", maxSkillFileBytes+1)
		}
		remote, _ := fakeGitHub(t, testSHA, tarball(t, headers, []string{body}))
		dest := filepath.Join(t.TempDir(), "o-r", testSHA)
		if err := remote.Fetch(context.Background(), testSHA, dest); err == nil {
			t.Fatalf("%s archive must be rejected", name)
		}
		if _, err := os.Stat(dest); err == nil {
			t.Fatalf("%s: partial extract left behind", name)
		}
	}
}

func TestLibraryCheckApplyAndFallback(t *testing.T) {
	archive := tarball(t, []tar.Header{
		{Name: "r-sha/tau-shell.yaml", Typeflag: tar.TypeReg},
		{Name: "r-sha/skills/alpha/SKILL.md", Typeflag: tar.TypeReg},
	}, []string{"version: 1\nskills: [alpha]\n", goodSkill})
	remote, _ := fakeGitHub(t, testSHA, archive)
	library, err := NewLibrary(t.TempDir(), DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	library.remote = remote
	if err := library.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if status := library.Status(); !status.UpdateAvailable || status.Source != SourceBundled {
		t.Fatalf("before apply: %+v", status)
	}
	if err := library.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := library.Status()
	if status.UpdateAvailable || status.Source != SourceCookbook || status.Applied != short(testSHA) {
		t.Fatalf("after apply: %+v", status)
	}
	found := false
	for _, entry := range library.List("claude-code") {
		found = found || (entry.Name == "alpha" && entry.Source == SourceCookbook)
	}
	if !found {
		t.Fatal("applied cookbook skill missing from list")
	}
	remote.API = "http://127.0.0.1:1"
	remote.Client.Timeout = 0
	if err := library.Check(context.Background()); err == nil {
		t.Fatal("offline check must report failure")
	}
	if status := library.Status(); status.Source != SourceCookbook || status.Message == "" {
		t.Fatalf("offline must keep applied snapshot and say why: %+v", status)
	}
}
