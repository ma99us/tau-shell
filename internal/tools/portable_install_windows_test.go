//go:build windows

package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestParseLatestNodeLTS(t *testing.T) {
	index := []byte(`[
{"version":"v27.0.0-alpha.0.0.0","files":["win-x64-zip"],"lts":false},
{"version":"v26.13.0","files":["win-x64-zip"],"lts":"Lithium"},
{"version":"v24.22.2","files":["win-x64-zip"],"lts":"Krypton"}
]`)
	release, err := parseLatestNodeLTS(index)
	if err != nil || release.Version != "v26.13.0" {
		t.Fatalf("latest Node LTS = %+v, %v", release, err)
	}
	if _, err := parseLatestNodeLTS([]byte(`[{"version":"v27.0.0","files":["win-x64-zip"],"lts":false}]`)); err == nil {
		t.Fatal("accepted an index with no LTS version")
	}
}

func TestParseLatestPortableGit(t *testing.T) {
	metadata := []byte(`{"tag_name":"v2.56.0.windows.4","draft":false,"prerelease":false,"assets":[{"name":"PortableGit-2.56.0.4-64-bit.7z.exe","browser_download_url":"https://github.com/git-for-windows/git/releases/download/v2.56.0.windows.4/PortableGit-2.56.0.4-64-bit.7z.exe","digest":"sha256:b1f3e52216e2ed6882d1bebfd4d3fdf319bec40d2edbbf9fd26ec4fd3fce7ed1"},{"name":"Git-2.56.0.4-64-bit.tar.bz2","browser_download_url":"https://github.com/git-for-windows/git/releases/download/v2.56.0.windows.4/Git-2.56.0.4-64-bit.tar.bz2","digest":"sha256:0dae059764ba7c447c9604eb6cb0f4afaf2d90b003ecf1c8cf2dbe5a13e4442c"}]}`)
	release, asset, digest, err := parseLatestPortableGit(metadata)
	if err != nil || release.TagName != "v2.56.0.windows.4" || asset.Name != "Git-2.56.0.4-64-bit.tar.bz2" || digest != "0dae059764ba7c447c9604eb6cb0f4afaf2d90b003ecf1c8cf2dbe5a13e4442c" {
		t.Fatalf("portable Git selection = %+v, %+v, %s, %v", release, asset, digest, err)
	}
	metadata = bytes.Replace(metadata, []byte("https://github.com/git-for-windows/git/releases/download/v2.56.0.windows.4/Git-2.56.0.4-64-bit.tar.bz2"), []byte("https://example.invalid/git.tar.bz2"), 1)
	if _, _, _, err := parseLatestPortableGit(metadata); err == nil {
		t.Fatal("accepted a PortableGit asset URL outside the official release")
	}
}

func TestParseNodeChecksum(t *testing.T) {
	manifest := []byte("aabbccdd" + strings.Repeat("0", 56) + "  node-v26.13.0-win-x64.zip\n")
	got, err := parseNodeChecksum(manifest, "node-v26.13.0-win-x64.zip")
	if err != nil || got != "aabbccdd"+strings.Repeat("0", 56) {
		t.Fatalf("checksum = %s, %v", got, err)
	}
	if _, err := parseNodeChecksum(manifest, "node-v26.13.0-win-arm64.zip"); err == nil {
		t.Fatal("accepted missing checksum entry")
	}
}

func TestSafeArchivePath(t *testing.T) {
	for _, name := range []string{"../outside", "a/../../outside", `/absolute/file`, `C:\outside`, "a/.."} {
		if _, err := safeArchivePath(name); err == nil {
			t.Errorf("accepted unsafe archive path %q", name)
		}
	}
	if got, err := safeArchivePath("node/bin/node.exe"); err != nil || got != "node/bin/node.exe" {
		t.Fatalf("safe path = %q, %v", got, err)
	}
}

func TestExtractNodeZipAndRejectsTraversal(t *testing.T) {
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "node.zip")
	createZip(t, archivePath, map[string]string{
		"node-v26.13.0-win-x64/node.exe": "node",
		"node-v26.13.0-win-x64/npm.cmd":  "npm",
	})
	destination := filepath.Join(directory, "stage")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := extractNodeZip(archivePath, "v26.13.0", destination); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(destination, "node.exe")) || !exists(filepath.Join(destination, "npm.cmd")) {
		t.Fatal("Node ZIP files were not extracted")
	}

	badArchive := filepath.Join(directory, "bad.zip")
	createZip(t, badArchive, map[string]string{"node-v26.13.0-win-x64/../../escape.txt": "escape"})
	if err := extractNodeZip(badArchive, "v26.13.0", filepath.Join(directory, "bad-stage")); err == nil {
		t.Fatal("accepted path traversal in Node ZIP")
	}
}

func TestExtractPortableGitTarCopiesHardlinksAndSkipsVirtualDevices(t *testing.T) {
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	writeTarFile(t, writer, "cmd/git.exe", []byte("portable git"))
	if err := writer.WriteHeader(&tar.Header{Name: "ucrt64/bin/git.exe", Linkname: "cmd/git.exe", Typeflag: tar.TypeLink, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteHeader(&tar.Header{Name: "dev/fd", Linkname: "/proc/self/fd", Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := extractPortableGitTar(bytes.NewReader(data.Bytes()), destination); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmd/git.exe", "ucrt64/bin/git.exe"} {
		got, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(name)))
		if err != nil || string(got) != "portable git" {
			t.Fatalf("extracted %s = %q, %v", name, got, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(destination, "dev", "fd")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("virtual device symlink was unexpectedly materialized: %v", err)
	}
}

func TestExtractPortableGitTarRejectsUnexpectedLinksAndTraversal(t *testing.T) {
	for _, header := range []*tar.Header{
		{Name: "escape", Linkname: "../../outside", Typeflag: tar.TypeLink},
		{Name: "unexpected/link", Linkname: "../target", Typeflag: tar.TypeSymlink},
		{Name: "../../outside", Typeflag: tar.TypeReg, Size: 1},
	} {
		var data bytes.Buffer
		writer := tar.NewWriter(&data)
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := writer.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := extractPortableGitTar(bytes.NewReader(data.Bytes()), t.TempDir()); err == nil {
			t.Errorf("accepted unsafe tar entry %+v", header)
		}
	}
}

func TestLiveManagedPortableInstalls(t *testing.T) {
	if os.Getenv("TAUSHELL_LIVE_PORTABLE_INSTALLS") != "1" {
		t.Skip("set TAUSHELL_LIVE_PORTABLE_INSTALLS=1 to download official Node.js and Git for Windows archives into a temporary directory")
	}
	oldPath, oldPrefix := os.Getenv("PATH"), os.Getenv("NPM_CONFIG_PREFIX")
	userRegistryPath := readRegistryPath(t, registry.CURRENT_USER, `Environment`)
	machineRegistryPath := readRegistryPath(t, registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		if oldPrefix == "" {
			_ = os.Unsetenv("NPM_CONFIG_PREFIX")
		} else {
			_ = os.Setenv("NPM_CONFIG_PREFIX", oldPrefix)
		}
	})
	t.Cleanup(func() {
		if got := readRegistryPath(t, registry.CURRENT_USER, `Environment`); got != userRegistryPath {
			t.Errorf("user PATH registry value changed: before %q, after %q", userRegistryPath, got)
		}
		if got := readRegistryPath(t, registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`); got != machineRegistryPath {
			t.Errorf("machine PATH registry value changed: before %q, after %q", machineRegistryPath, got)
		}
	})
	layout := ManagedToolLayout{Root: filepath.Join(t.TempDir(), "TauShell Tools - \u03bb", "tools")}
	ids := []ID{NodeJS, Git}
	if os.Getenv("TAUSHELL_LIVE_PORTABLE_GIT_ONLY") == "1" {
		ids = []ID{Git}
	}
	for _, id := range ids {
		spec, ok := Lookup(id)
		if !ok {
			t.Fatalf("tool %s was not declared", id)
		}
		message, err := InstallManagedTool(t.Context(), layout, spec)
		if err != nil {
			t.Fatalf("install %s: %v", id, err)
		}
		t.Log(message)
		status := Detect(t.Context(), func() Environment {
			environment := DefaultEnvironment()
			environment.ManagedRoots = layout.ManagedRoots()
			return environment
		}(), spec)
		if !status.Usable() || !status.Managed {
			t.Fatalf("managed %s status = %+v", id, status)
		}
	}
	includes := func(id ID) bool {
		for _, current := range ids {
			if current == id {
				return true
			}
		}
		return false
	}
	if includes(NodeJS) {
		npm, err := exec.LookPath("npm.cmd")
		if err != nil || !IsManagedPath(npm, layout.ManagedRoots()) {
			t.Fatalf("managed npm lookup = %q, %v", npm, err)
		}
		fixture := filepath.Join(t.TempDir(), "npm fixture")
		binDirectory := filepath.Join(fixture, "bin")
		if err := os.MkdirAll(binDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		packageJSON := `{"name":"taushell-portable-smoke","version":"1.0.0","bin":{"taushell-smoke":"bin/fixture.js"}}`
		if err := os.WriteFile(filepath.Join(fixture, "package.json"), []byte(packageJSON), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDirectory, "fixture.js"), []byte("#!/usr/bin/env node\nconsole.log('managed npm fixture');\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := RunNpm(t.Context(), "install", "--global", "--prefix", layout.NpmPrefix(), "--ignore-scripts", fixture); err != nil {
			t.Fatalf("install local npm fixture: %v", err)
		}
		shim, err := exec.LookPath("taushell-smoke.cmd")
		if err != nil || !IsManagedPath(shim, layout.ManagedRoots()) {
			t.Fatalf("managed npm shim lookup = %q, %v", shim, err)
		}
		pwsh, err := exec.LookPath("pwsh.exe")
		if err != nil {
			t.Skip("PowerShell 7 is not installed for the managed npm shim smoke")
		}
		command := exec.CommandContext(t.Context(), pwsh, "-NoLogo", "-NoProfile", "-Command", "& '"+strings.ReplaceAll(shim, "'", "''")+"'")
		if output, err := command.CombinedOutput(); err != nil || !strings.Contains(string(output), "managed npm fixture") {
			t.Fatalf("run managed npm shim: %v: %s", err, output)
		}
	}
	if includes(Git) {
		project := filepath.Join(t.TempDir(), "git init project")
		if err := os.MkdirAll(project, 0o700); err != nil {
			t.Fatal(err)
		}
		gitPath, err := exec.LookPath("git.exe")
		if err != nil || !IsManagedPath(gitPath, layout.ManagedRoots()) {
			t.Fatalf("managed git lookup = %q, %v", gitPath, err)
		}
		command := exec.CommandContext(t.Context(), gitPath, "init")
		command.Dir = project
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=NUL")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("managed git init: %v: %s", err, output)
		}
	}
}

func readRegistryPath(t *testing.T, root registry.Key, keyPath string) string {
	t.Helper()
	key, err := registry.OpenKey(root, keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("open PATH registry key %s: %v", keyPath, err)
	}
	defer key.Close()
	value, _, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		t.Fatalf("read PATH registry value %s: %v", keyPath, err)
	}
	return value
}

func createZip(t *testing.T, filename string, files map[string]string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, contents := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTarFile(t *testing.T, writer *tar.Writer, name string, contents []byte) {
	t.Helper()
	if err := writer.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(contents); err != nil {
		t.Fatal(err)
	}
}
