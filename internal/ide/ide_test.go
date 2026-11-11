package ide

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func touch(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func fake(ids ...string) []Installed {
	var items []Installed
	for _, id := range ids {
		def, ok := Lookup(id)
		if !ok {
			panic(id)
		}
		items = append(items, Installed{Def: def, Exe: `C:\ide\` + id + ".exe"})
	}
	return items
}

func ids(matches []Match) []string {
	out := []string{}
	for _, match := range matches {
		out = append(out, match.ID)
	}
	return out
}

func TestCandidatesRequireMarkerExceptVSCodeFamily(t *testing.T) {
	root := t.TempDir()
	got := Candidates(root, fake("vscode", "cursor", "idea", "visualstudio", "eclipse"), true)
	if want := []string{"vscode", "cursor"}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("empty folder offered %v, want %v", ids(got), want)
	}
	touch(t, root, "App.sln", ".project", ".idea/misc.xml", "pom.xml")
	got = Candidates(root, fake("vscode", "cursor", "idea", "visualstudio", "eclipse"), true)
	want := []string{"idea", "visualstudio", "eclipse", "vscode", "cursor"}
	if !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("marked folder offered %v, want %v", ids(got), want)
	}
	if !got[0].Matched || got[3].Matched {
		t.Fatalf("matched flags wrong: %+v", got)
	}
}

func TestJetBrainsHeuristicNarrowsThenFallsBack(t *testing.T) {
	root := t.TempDir()
	touch(t, root, ".idea/misc.xml", "go.mod")
	installed := fake("idea", "goland", "pycharm")
	if got := ids(Candidates(root, installed, true)); !reflect.DeepEqual(got, []string{"goland"}) {
		t.Fatalf("go project offered %v", got)
	}
	// Product not installed: offer every installed JetBrains IDE.
	if got := ids(Candidates(root, fake("idea", "pycharm"), true)); !reflect.DeepEqual(got, []string{"idea", "pycharm"}) {
		t.Fatalf("fallback offered %v", got)
	}
	other := t.TempDir()
	touch(t, other, "go.mod")
	if got := Candidates(other, installed, true); len(got) != 0 {
		t.Fatalf("no .idea marker must offer no JetBrains IDE, got %v", ids(got))
	}
}

func TestJetBrainsCandidatesDetectsRiderAndAndroid(t *testing.T) {
	root := t.TempDir()
	touch(t, root, ".idea/.idea.App/.idea/misc.xml")
	if got := JetBrainsCandidates(root); len(got) == 0 || got[0] != "rider" {
		t.Fatalf("rider not first: %v", got)
	}
	android := t.TempDir()
	touch(t, android, "app/src/main/AndroidManifest.xml", "build.gradle")
	if got := JetBrainsCandidates(android); got[0] != "android-studio" || got[len(got)-1] != "idea" {
		t.Fatalf("android project: %v", got)
	}
}

func TestWSLOffersOnlyVSCodeUnscanned(t *testing.T) {
	got := Candidates("/home/me/app", fake("vscode", "cursor", "idea", "visualstudio"), false)
	if !reflect.DeepEqual(ids(got), []string{"vscode"}) || got[0].Matched {
		t.Fatalf("WSL offered %+v", got)
	}
}

func TestBuildCommand(t *testing.T) {
	root := t.TempDir()
	vscode := fake("vscode")[0]
	command, err := BuildCommand(vscode, root, "")
	if err != nil || !reflect.DeepEqual(command.Args, []string{"-n", root}) {
		t.Fatalf("vscode: %v %v", command, err)
	}
	command, err = BuildCommand(vscode, "/home/me/app", "Ubuntu")
	if err != nil || !reflect.DeepEqual(command.Args, []string{"-n", "--remote", "wsl+Ubuntu", "/home/me/app"}) {
		t.Fatalf("wsl: %v %v", command, err)
	}
	if _, err := BuildCommand(fake("idea")[0], "/home/me/app", "Ubuntu"); err == nil {
		t.Fatal("JetBrains must refuse WSL")
	}
	vs := fake("visualstudio")[0]
	touch(t, root, "App.sln")
	if command, _ := BuildCommand(vs, root, ""); command.Args[0] != filepath.Join(root, "App.sln") {
		t.Fatalf("single solution not used: %v", command.Args)
	}
	touch(t, root, "Other.slnx")
	if command, _ := BuildCommand(vs, root, ""); command.Args[0] != root {
		t.Fatalf("several solutions must open the folder: %v", command.Args)
	}
	if command, _ := BuildCommand(fake("eclipse")[0], root, ""); command.Note == "" || len(command.Args) != 0 {
		t.Fatalf("eclipse must warn and pass no args: %+v", command)
	}
}

func TestLocatePicksNewestVersionedFolder(t *testing.T) {
	programs := t.TempDir()
	t.Setenv("ProgramFiles", programs)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	for _, version := range []string{"IntelliJ IDEA 2024.3.1", "IntelliJ IDEA 2025.1"} {
		touch(t, programs, filepath.Join("JetBrains", version, "bin", "idea64.exe"))
	}
	def, _ := Lookup("idea")
	if got, want := Locate(def), filepath.Join(programs, "JetBrains", "IntelliJ IDEA 2025.1", "bin", "idea64.exe"); got != want {
		t.Fatalf("Locate = %q, want %q", got, want)
	}
}

func TestLocateUsesUninstallEntry(t *testing.T) {
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	install := t.TempDir()
	touch(t, install, filepath.Join("bin", "pycharm64.exe"))
	previous := uninstallEntries
	uninstallEntries = func() []UninstallEntry {
		return []UninstallEntry{{DisplayName: "PyCharm 2025.1", InstallLocation: install}}
	}
	defer func() { uninstallEntries = previous }()
	def, _ := Lookup("pycharm")
	if got := Locate(def); got != filepath.Join(install, "bin", "pycharm64.exe") {
		t.Fatalf("Locate = %q", got)
	}
}
