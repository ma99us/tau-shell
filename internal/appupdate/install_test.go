package appupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestNewReleaseKeepsOnlyTrustedAssets(t *testing.T) {
	release := newRelease(apiRelease{Tag: "v1.2.3", URL: ReleasesPrefix + "/tag/v1.2.3", Body: "notes", Assets: []apiAsset{
		{Name: "TauShell-1.2.3-windows-x64.zip", URL: DownloadPrefix + "v1.2.3/TauShell-1.2.3-windows-x64.zip"},
		{Name: SumsName, URL: "https://evil.example/SHA256SUMS.txt"},
	}})
	if release.ZipURL == "" || release.SumsURL != "" || release.Notes != "notes" {
		t.Fatalf("unexpected release %+v", release)
	}
}

func TestDownloadVerifiesChecksum(t *testing.T) {
	archive := buildZip(t, map[string]string{"taushell.exe": "x"})
	sum := sha256.Sum256(archive)
	name := "TauShell-1.2.3-windows-x64.zip"
	sums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	badSums := strings.Repeat("0", 64) + "  " + name + "\n"
	for _, tc := range []struct {
		sums    string
		wantErr bool
	}{{sums, false}, {badSums, true}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, SumsName) {
				_, _ = w.Write([]byte(tc.sums))
				return
			}
			_, _ = w.Write(archive)
		}))
		client := server.Client()
		client.Transport = rewrite{server.URL, client.Transport}
		dir := t.TempDir()
		path, err := Download(context.Background(), client, Release{
			ZipURL: DownloadPrefix + "v1.2.3/" + name, SumsURL: DownloadPrefix + "v1.2.3/" + SumsName,
		}, dir, nil)
		server.Close()
		if (err != nil) != tc.wantErr {
			t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
		}
		if tc.wantErr {
			if _, statErr := os.Stat(filepath.Join(dir, name)); statErr == nil {
				t.Fatal("mismatched download was kept")
			}
		} else if path == "" {
			t.Fatal("no path returned")
		}
	}
}

// rewrite sends the fixed GitHub URLs to the test server.
type rewrite struct {
	target string
	next   http.RoundTripper
}

func (r rewrite) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Scheme = "http"
	clone.URL.Host = strings.TrimPrefix(r.target, "http://")
	return r.next.RoundTrip(clone)
}

func TestFetchRefusesForeignURLAndOversize(t *testing.T) {
	if err := fetch(context.Background(), http.DefaultClient, "https://evil.example/a.zip", 10, &bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected foreign URL to be refused")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("a"), 100))
	}))
	defer server.Close()
	client := server.Client()
	client.Transport = rewrite{server.URL, client.Transport}
	if err := fetch(context.Background(), client, DownloadPrefix+"x", 10, &bytes.Buffer{}, nil); err == nil {
		t.Fatal("expected size cap error")
	}
}

func TestExtractRejectsZipSlipAndNeedsExe(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := Extract(write("slip.zip", buildZip(t, map[string]string{"../evil.txt": "x", "taushell.exe": "x"})), filepath.Join(dir, "a")); err == nil {
		t.Fatal("expected zip-slip rejection")
	}
	if err := Extract(write("noexe.zip", buildZip(t, map[string]string{"a.txt": "x"})), filepath.Join(dir, "b")); err == nil {
		t.Fatal("expected missing exe error")
	}
	if err := Extract(write("ok.zip", buildZip(t, map[string]string{"taushell.exe": "new", "sub/a.dll": "d"})), filepath.Join(dir, "c")); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApplyAndRollback(t *testing.T) {
	install := t.TempDir()
	staged := filepath.Join(install, UpdateDir, "1.1.0")
	for path, content := range map[string]string{filepath.Join(install, "taushell.exe"): "old", filepath.Join(staged, "taushell.exe"): "new", filepath.Join(staged, "extra.dll"): "e"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Apply(install, staged, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(install, "taushell.exe")) != "new" {
		t.Fatal("update not installed")
	}
	if version, ok := RollbackVersion(install); !ok || version != "1.0.0" {
		t.Fatalf("rollback version = %q %v", version, ok)
	}
	if err := Rollback(install, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(install, "taushell.exe")) != "old" {
		t.Fatal("rollback did not restore the old exe")
	}
	if version, _ := RollbackVersion(install); version != "1.1.0" {
		t.Fatalf("rollback copy = %q, want 1.1.0", version)
	}
}

func TestInstallUndoesOnFailure(t *testing.T) {
	install := t.TempDir()
	staged := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "taushell.exe"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "taushell.exe"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A file where a staged subfolder must go makes the second move fail.
	if err := os.WriteFile(filepath.Join(install, "zsub"), []byte("f"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staged, "zsub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "zsub", "a.dll"), []byte("z"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Install(install, staged, filepath.Join(install, RollbackDir, "1.0.0")); err == nil {
		t.Fatal("expected install failure")
	}
	if read(t, filepath.Join(install, "taushell.exe")) != "old" {
		t.Fatal("failed install did not restore the old exe")
	}
}

func TestResultRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r", "update-result.json")
	if result, err := TakeResult(path); err != nil || result != nil {
		t.Fatalf("empty = %v %v", result, err)
	}
	if err := WriteResult(path, Result{From: "1.0.0", To: "1.1.0", Sessions: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	result, err := TakeResult(path)
	if err != nil || result == nil || result.To != "1.1.0" || len(result.Sessions) != 1 {
		t.Fatalf("result = %+v %v", result, err)
	}
	if again, _ := TakeResult(path); again != nil {
		t.Fatal("result was not cleared")
	}
}

func TestLatestReturnsNotesAndAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v1.4.0","html_url":"` + ReleasesPrefix + `/tag/v1.4.0","body":"What changed","assets":[
			{"name":"TauShell-1.4.0-windows-x64.zip","browser_download_url":"` + DownloadPrefix + `v1.4.0/TauShell-1.4.0-windows-x64.zip"},
			{"name":"SHA256SUMS.txt","browser_download_url":"` + DownloadPrefix + `v1.4.0/SHA256SUMS.txt"}]}]`))
	}))
	defer server.Close()
	release, ok, err := Latest(context.Background(), server.Client(), server.URL)
	if err != nil || !ok || release.Notes != "What changed" || release.ZipURL == "" || release.SumsURL == "" {
		t.Fatalf("release = %+v ok=%v err=%v", release, ok, err)
	}
}

func TestChecksumFor(t *testing.T) {
	hash := strings.Repeat("a", 64)
	sums := hash + "  other.zip\n" + hash[:62] + "b *want.zip\r\nshort  bad.zip\n"
	if _, ok := checksumFor(sums, "other.zip"); !ok {
		t.Fatal("expected other.zip")
	}
	if got, ok := checksumFor(sums, "want.zip"); ok {
		t.Fatalf("63-char hash must be rejected, got %q", got)
	}
	if _, ok := checksumFor(sums, "missing.zip"); ok {
		t.Fatal("unexpected match")
	}
}

func TestSafeNameStripsPathCharacters(t *testing.T) {
	cases := map[string]string{"v1.0.1": "1.0.1", "1.0.0-rc1": "1.0.0-rc1", "..\\x/y": "..xy", "": "previous", "***": "previous"}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyKeepsOnlyOneRollbackCopy(t *testing.T) {
	install := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "taushell.exe"), []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ from, content string }{{"1.0.0", "v2"}, {"1.1.0", "v3"}} {
		staged := t.TempDir()
		if err := os.WriteFile(filepath.Join(staged, "taushell.exe"), []byte(step.content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Apply(install, staged, step.from); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(install, RollbackDir))
	if err != nil || len(entries) != 1 || entries[0].Name() != "1.1.0" {
		t.Fatalf("rollback entries = %v err=%v", entries, err)
	}
}

func TestRollbackWithoutCopyFails(t *testing.T) {
	if err := Rollback(t.TempDir(), "1.0.0"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDownloadRejectsMissingAssets(t *testing.T) {
	if _, err := Download(context.Background(), http.DefaultClient, Release{}, t.TempDir(), nil); err == nil {
		t.Fatal("expected error for release without assets")
	}
}
