// Package ide finds the IDEs installed on this machine, decides which of them
// suit a project folder, and builds the command that opens the folder in one.
// Only IDEs in the built-in catalog can be launched, and always with the
// executable the host located itself.
package ide

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Family groups IDEs that share a command line shape.
type Family string

const (
	FamilyVSCode       Family = "vscode"
	FamilyJetBrains    Family = "jetbrains"
	FamilyVisualStudio Family = "visualstudio"
	FamilyEclipse      Family = "eclipse"
)

// Def describes one IDE the host can detect and launch.
type Def struct {
	ID     string
	Name   string
	Family Family
	// Exes are glob templates (with %VAR% references) that locate the
	// executable. When several match, the last one in sorted order wins, which
	// is the newest version for versioned install folders.
	Exes []string
	// PathExe is an executable name looked up on PATH as a last resort.
	PathExe string
	// Uninstall matches a Windows uninstall entry whose DisplayName starts with
	// one of these prefixes; UninstallExe is then joined to InstallLocation.
	Uninstall    []string
	UninstallExe string
	// Markers are root-folder entries (glob patterns) that tie a project to the IDE.
	Markers []string
	// Always offers the IDE for any folder when it is installed.
	Always bool
	// WSL is true when the IDE can open a folder inside a WSL distro.
	WSL bool
}

func jetBrains(id, name, exe string, folders ...string) Def {
	def := Def{
		ID: id, Name: name, Family: FamilyJetBrains, UninstallExe: `bin\` + exe,
		Markers: []string{".idea", "*.iml"}, Uninstall: []string{name},
	}
	for _, folder := range folders {
		def.Exes = append(def.Exes,
			fmt.Sprintf(`%%ProgramFiles%%\JetBrains\%s*\bin\%s`, folder, exe),
			fmt.Sprintf(`%%LOCALAPPDATA%%\Programs\%s*\bin\%s`, folder, exe),
			fmt.Sprintf(`%%LOCALAPPDATA%%\JetBrains\Toolbox\apps\%s*\ch-*\*\bin\%s`, folder, exe),
		)
	}
	return def
}

// Catalog lists every supported IDE in display order.
var Catalog = []Def{
	{ID: "vscode", Name: "VS Code", Family: FamilyVSCode, Always: true, WSL: true,
		Exes:      []string{`%LOCALAPPDATA%\Programs\Microsoft VS Code\Code.exe`, `%ProgramFiles%\Microsoft VS Code\Code.exe`},
		Uninstall: []string{"Microsoft Visual Studio Code"}, UninstallExe: "Code.exe",
		Markers: []string{".vscode", "*.code-workspace"}},
	{ID: "vscode-insiders", Name: "VS Code Insiders", Family: FamilyVSCode, Always: true, WSL: true,
		Exes:    []string{`%LOCALAPPDATA%\Programs\Microsoft VS Code Insiders\Code - Insiders.exe`, `%ProgramFiles%\Microsoft VS Code Insiders\Code - Insiders.exe`},
		Markers: []string{".vscode", "*.code-workspace"}},
	{ID: "cursor", Name: "Cursor", Family: FamilyVSCode, Always: true,
		Exes:      []string{`%LOCALAPPDATA%\Programs\cursor\Cursor.exe`, `%ProgramFiles%\cursor\Cursor.exe`},
		Uninstall: []string{"Cursor"}, UninstallExe: "Cursor.exe",
		Markers: []string{".vscode", ".cursor"}},
	jetBrains("idea", "IntelliJ IDEA", "idea64.exe", "IntelliJ IDEA", "IDEA"),
	jetBrains("pycharm", "PyCharm", "pycharm64.exe", "PyCharm"),
	jetBrains("webstorm", "WebStorm", "webstorm64.exe", "WebStorm"),
	jetBrains("goland", "GoLand", "goland64.exe", "GoLand"),
	jetBrains("rider", "Rider", "rider64.exe", "Rider", "JetBrains Rider"),
	jetBrains("clion", "CLion", "clion64.exe", "CLion"),
	jetBrains("rustrover", "RustRover", "rustrover64.exe", "RustRover"),
	jetBrains("phpstorm", "PhpStorm", "phpstorm64.exe", "PhpStorm"),
	jetBrains("rubymine", "RubyMine", "rubymine64.exe", "RubyMine"),
	{ID: "android-studio", Name: "Android Studio", Family: FamilyJetBrains, Markers: []string{".idea", "*.iml"},
		Exes:      []string{`%ProgramFiles%\Android\Android Studio*\bin\studio64.exe`, `%LOCALAPPDATA%\Programs\Android Studio*\bin\studio64.exe`, `%LOCALAPPDATA%\JetBrains\Toolbox\apps\AndroidStudio\ch-*\*\bin\studio64.exe`},
		Uninstall: []string{"Android Studio"}, UninstallExe: `bin\studio64.exe`},
	{ID: "visualstudio", Name: "Visual Studio", Family: FamilyVisualStudio,
		Exes:    []string{`%ProgramFiles%\Microsoft Visual Studio\*\*\Common7\IDE\devenv.exe`, `%ProgramFiles(x86)%\Microsoft Visual Studio\*\*\Common7\IDE\devenv.exe`},
		Markers: []string{"*.sln", "*.slnx", ".vs"}},
	{ID: "eclipse", Name: "Eclipse", Family: FamilyEclipse, PathExe: "eclipse.exe",
		Exes: []string{
			`%USERPROFILE%\eclipse\*\eclipse\eclipse.exe`, `%USERPROFILE%\eclipse\eclipse.exe`,
			`%ProgramFiles%\Eclipse*\eclipse.exe`, `%ProgramFiles%\eclipse\eclipse.exe`,
			`C:\eclipse\eclipse.exe`, `C:\eclipse\*\eclipse\eclipse.exe`,
		},
		Uninstall: []string{"Eclipse IDE", "Eclipse Foundation"}, UninstallExe: "eclipse.exe",
		Markers: []string{".project", ".classpath", ".settings"}},
}

// Lookup returns the catalog entry with the given ID.
func Lookup(id string) (Def, bool) {
	for _, def := range Catalog {
		if def.ID == id {
			return def, true
		}
	}
	return Def{}, false
}

// Installed is a catalog IDE found on this machine.
type Installed struct {
	Def
	Exe string
}

// Match is an installed IDE offered for a project folder.
type Match struct {
	Installed
	// Matched is true when the folder carries one of the IDE's own markers.
	Matched bool
}

// hasMarker reports whether any marker pattern matches an entry in root.
func hasMarker(root string, patterns []string) bool {
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(escapeGlob(root), pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}

func escapeGlob(path string) string {
	return strings.NewReplacer("[", "[[]").Replace(path)
}

func fileContains(path, needle string) bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1<<20 {
		return false
	}
	return strings.Contains(string(data), needle)
}

// JetBrainsCandidates guesses which JetBrains products suit the folder from
// well-known project files. The IDs are in priority order and may be empty.
func JetBrainsCandidates(root string) []string {
	exists := func(names ...string) bool {
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(root, name)); err == nil {
				return true
			}
		}
		return false
	}
	var ids []string
	add := func(id string, ok bool) {
		if ok {
			ids = append(ids, id)
		}
	}
	rider := false
	if entries, err := os.ReadDir(filepath.Join(root, ".idea")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), ".idea.") {
				rider = true
			}
		}
	}
	add("rider", rider)
	android := exists("AndroidManifest.xml", filepath.Join("app", "src", "main", "AndroidManifest.xml"))
	for _, name := range []string{"build.gradle", "build.gradle.kts", filepath.Join("app", "build.gradle"), filepath.Join("app", "build.gradle.kts")} {
		if fileContains(filepath.Join(root, name), "com.android.") {
			android = true
		}
	}
	add("android-studio", android)
	add("goland", exists("go.mod", "go.work"))
	add("pycharm", exists("pyproject.toml", "requirements.txt", "setup.py", "Pipfile"))
	add("clion", exists("CMakeLists.txt"))
	add("rustrover", exists("Cargo.toml"))
	add("phpstorm", exists("composer.json"))
	add("rubymine", exists("Gemfile"))
	add("webstorm", exists("package.json"))
	add("idea", exists("pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts") || hasMarker(root, []string{"*.iml"}))
	return ids
}

// Candidates picks which installed IDEs to offer for root. Markers decide
// relevance (VS Code family editors are always offered); matched IDEs sort
// first, otherwise the catalog order is kept. With scan false (a WSL project
// that must not be read from Windows) only IDEs that can open it are offered
// and none is marked as matched.
func Candidates(root string, installed []Installed, scan bool) []Match {
	jetBrainsOffered := map[string]bool{}
	if scan && hasMarker(root, []string{".idea", "*.iml"}) {
		var have []string
		for _, item := range installed {
			if item.Family == FamilyJetBrains {
				have = append(have, item.ID)
			}
		}
		for _, id := range JetBrainsCandidates(root) {
			for _, installedID := range have {
				if installedID == id {
					jetBrainsOffered[id] = true
				}
			}
		}
		if len(jetBrainsOffered) == 0 {
			for _, id := range have {
				jetBrainsOffered[id] = true
			}
		}
	}
	var matches []Match
	for _, item := range installed {
		if !scan && !item.WSL {
			continue
		}
		matched := scan && hasMarker(root, item.Markers)
		switch {
		case item.Family == FamilyJetBrains:
			if !jetBrainsOffered[item.ID] {
				continue
			}
		case item.Always:
		case !matched:
			continue
		}
		matches = append(matches, Match{Installed: item, Matched: matched})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Matched && !matches[j].Matched })
	return matches
}

// Command is how to start an IDE for a folder.
type Command struct {
	Exe  string
	Args []string
	// Note is shown to the user when the IDE cannot fully do what was asked.
	Note string
}

// BuildCommand returns the command that opens root in the IDE. For a WSL
// session root is the Linux path and distro names the distro; only IDEs with
// WSL support are accepted then.
func BuildCommand(item Installed, root, distro string) (Command, error) {
	if item.Exe == "" {
		return Command{}, fmt.Errorf("%s was not found on this machine", item.Name)
	}
	if root == "" {
		return Command{}, errors.New("the session has no project folder")
	}
	if distro != "" {
		if !item.WSL {
			return Command{}, fmt.Errorf("%s cannot open a WSL project", item.Name)
		}
		return Command{Exe: item.Exe, Args: []string{"-n", "--remote", "wsl+" + distro, root}}, nil
	}
	switch item.Family {
	case FamilyVSCode:
		return Command{Exe: item.Exe, Args: []string{"-n", root}}, nil
	case FamilyJetBrains:
		return Command{Exe: item.Exe, Args: []string{root}}, nil
	case FamilyVisualStudio:
		target := root
		var solutions []string
		for _, pattern := range []string{"*.sln", "*.slnx"} {
			found, _ := filepath.Glob(filepath.Join(escapeGlob(root), pattern))
			solutions = append(solutions, found...)
		}
		if len(solutions) == 1 {
			target = solutions[0]
		}
		return Command{Exe: item.Exe, Args: []string{target}}, nil
	case FamilyEclipse:
		// Eclipse has no command-line way to import a folder as a project.
		return Command{Exe: item.Exe, Note: "Eclipse cannot open a folder from the command line. In Eclipse use File > Open Projects from File System and choose " + root}, nil
	}
	return Command{}, fmt.Errorf("unsupported IDE %q", item.ID)
}
