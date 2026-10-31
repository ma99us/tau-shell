//go:build windows

package tools

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestManagedToolPathEntriesUseActivePerUserInstalls(t *testing.T) {
	layout := ManagedToolLayout{Root: filepath.Join(t.TempDir(), "TauShell", "tools")}
	nodeDir := layout.NodeVersionDir("26.13.0")
	npmPrefix := layout.NpmPrefix()
	gitCommandDir := filepath.Join(layout.GitVersionDir("2.56.0.windows.4"), "cmd")
	for _, file := range []string{
		filepath.Join(nodeDir, "node.exe"),
		filepath.Join(nodeDir, "npm.cmd"),
		filepath.Join(npmPrefix, "gsd.cmd"),
		filepath.Join(gitCommandDir, "git.exe"),
	} {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := layout.SetActiveVersion(NodeJS, "26.13.0"); err != nil {
		t.Fatal(err)
	}
	if err := layout.SetActiveVersion(Git, "2.56.0.windows.4"); err != nil {
		t.Fatal(err)
	}
	entries := layout.PathEntries()
	want := []string{npmPrefix, nodeDir, gitCommandDir}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("managed PATH entries = %v, want %v", entries, want)
	}
	current := `C:\Windows;C:\Program Files\Git\cmd;C:\windows\`
	path := layout.ProcessPath(current)
	if got := strings.Split(path, ";"); !reflect.DeepEqual(got, []string{npmPrefix, nodeDir, gitCommandDir, `C:\Windows`, `C:\Program Files\Git\cmd`}) {
		t.Fatalf("managed PATH = %q", path)
	}
	if !IsManagedPath(filepath.Join(nodeDir, "node.exe"), layout.ManagedRoots()) ||
		!IsManagedPath(filepath.Join(gitCommandDir, "git.exe"), layout.ManagedRoots()) ||
		IsManagedPath(`C:\Windows\System32\git.exe`, layout.ManagedRoots()) {
		t.Fatal("managed PATH ownership check returned unexpected results")
	}
}

func TestSetActiveVersionReplacesExistingMarker(t *testing.T) {
	layout := ManagedToolLayout{Root: t.TempDir()}
	for _, version := range []string{"26.13.0", "26.13.1"} {
		if err := layout.SetActiveVersion(NodeJS, version); err != nil {
			t.Fatalf("activate %s: %v", version, err)
		}
		got, err := layout.ActiveVersion(NodeJS)
		if err != nil || got != version {
			t.Fatalf("active version = %q, %v; want %q", got, err, version)
		}
	}
}

func TestManagedToolPathIgnoresMissingOrInvalidInstalls(t *testing.T) {
	layout := ManagedToolLayout{Root: filepath.Join(t.TempDir(), "tools")}
	if entries := layout.PathEntries(); len(entries) != 0 {
		t.Fatalf("entries without markers = %v", entries)
	}
	if err := os.MkdirAll(layout.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(layout.Root, "node-version"), []byte("../../outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := layout.ActiveVersion(NodeJS); err == nil {
		t.Fatal("accepted path traversal in active version")
	}
	if err := layout.SetActiveVersion(NodeJS, "../outside"); err == nil {
		t.Fatal("wrote invalid active version")
	}
	if err := layout.SetActiveVersion(NodeJS, "26.13.0"); err != nil {
		t.Fatal(err)
	}
	if entries := layout.PathEntries(); len(entries) != 0 {
		t.Fatalf("entries for incomplete install = %v", entries)
	}
}

func TestPrependManagedPathLeavesInheritedPathAloneWithoutManagedTools(t *testing.T) {
	layout := ManagedToolLayout{Root: filepath.Join(t.TempDir(), "tools")}
	oldPath, oldPrefix := os.Getenv("PATH"), os.Getenv("NPM_CONFIG_PREFIX")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		if oldPrefix == "" {
			_ = os.Unsetenv("NPM_CONFIG_PREFIX")
		} else {
			_ = os.Setenv("NPM_CONFIG_PREFIX", oldPrefix)
		}
	})
	wantPath := `C:\Tools;;C:\Windows\`
	if err := os.Setenv("PATH", wantPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("NPM_CONFIG_PREFIX", `C:\User\npm`); err != nil {
		t.Fatal(err)
	}
	if err := layout.PrependToProcessPath(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("PATH"); got != wantPath {
		t.Fatalf("PATH changed without managed tools: %q, want %q", got, wantPath)
	}
	if got := os.Getenv("NPM_CONFIG_PREFIX"); got != `C:\User\npm` {
		t.Fatalf("NPM_CONFIG_PREFIX changed without managed Node: %q", got)
	}
}

func TestNpmCLIPathResolvesBesideNodeInUnicodePath(t *testing.T) {
	nodeDirectory := filepath.Join(t.TempDir(), "Node Tools - \u03bb")
	nodePath := filepath.Join(nodeDirectory, "node.exe")
	npmPath := filepath.Join(nodeDirectory, "npm.cmd")
	cliPath := filepath.Join(nodeDirectory, "node_modules", "npm", "bin", "npm-cli.js")
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := npmCLIPath(nodePath, npmPath)
	if err != nil || got != cliPath {
		t.Fatalf("npmCLIPath = %q, %v; want %q", got, err, cliPath)
	}
}

func TestManagedNodeSetsOnlyProcessNpmPrefix(t *testing.T) {
	layout := ManagedToolLayout{Root: filepath.Join(t.TempDir(), "tools")}
	nodeDir := layout.NodeVersionDir("26.13.0")
	for _, name := range []string{"node.exe", "npm.cmd"} {
		if err := os.MkdirAll(nodeDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nodeDir, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := layout.SetActiveVersion(NodeJS, "26.13.0"); err != nil {
		t.Fatal(err)
	}
	oldPath, oldPrefix := os.Getenv("PATH"), os.Getenv("NPM_CONFIG_PREFIX")
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		if oldPrefix == "" {
			_ = os.Unsetenv("NPM_CONFIG_PREFIX")
		} else {
			_ = os.Setenv("NPM_CONFIG_PREFIX", oldPrefix)
		}
	})
	if err := os.Setenv("PATH", `C:\Windows`); err != nil {
		t.Fatal(err)
	}
	if err := layout.PrependToProcessPath(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("NPM_CONFIG_PREFIX"); got != layout.NpmPrefix() {
		t.Fatalf("NPM_CONFIG_PREFIX = %q, want %q", got, layout.NpmPrefix())
	}
	if !strings.HasPrefix(os.Getenv("PATH"), layout.NodeVersionDir("26.13.0")) {
		t.Fatalf("managed Node was not prepended to PATH: %q", os.Getenv("PATH"))
	}
}
