# 12. IDE integration (open the project in an IDE)

Status: built (first version). This page records the research, the decisions
and what was and was not verified. User-facing behavior is in the
[README](../../../README.md); the launch security rules are in the
[security model](../../v2/11-security-model.md#ide-launcher).

## What it does

The session top bar shows one split button next to the project path. The main
part opens the session working folder in the last IDE used for the session
(then the last IDE used anywhere, then the first option). The caret opens a
list of every offered IDE. The icon is extracted at runtime from the
installed executable; no logos are bundled.

## When an IDE is offered

An IDE is offered only when the host both finds its marker in the working
folder root and finds the IDE installed. VS Code-family editors are the
exception: they are offered whenever installed, because they open any folder.

| IDE | Marker in root | Command used | Notes |
|---|---|---|---|
| VS Code, VS Code Insiders | any folder (also `.vscode`) | `code -n <dir>` | WSL: `--remote wsl+<distro>` |
| Cursor | any folder | `cursor -n <dir>` | no WSL support offered |
| JetBrains (IDEA, PyCharm, WebStorm, GoLand, Rider, CLion, RustRover, PhpStorm, RubyMine) and Android Studio | `.idea/` or `*.iml` | `<ide>64.exe <dir>` | new-window behavior follows the IDE's own setting |
| Visual Studio | `*.sln` or `*.slnx` | `devenv.exe <sln>` or `<dir>` | one solution file is passed; otherwise the folder is opened |
| Eclipse | `.project`, `.classpath` or `.settings/` | `eclipse.exe` | no documented command line imports a folder; a note tells the user to use File > Open Projects from File System |

JetBrains products are guessed from project files (for example `go.mod` means
GoLand, `package.json` means WebStorm, `.idea/.idea.*` means Rider). When none
of the guessed products is installed, every installed JetBrains IDE is offered.

Installed detection checks known install paths (environment-expanded globs,
newest version wins), then Windows uninstall entries, then PATH. Results are
cached for one minute.

## Decisions

- One split button with a remembered pick, not one button per IDE.
- Strict rule (marker and installed), except VS Code-family editors.
- Folder is the session working directory.
- WSL sessions: VS Code and Insiders only, through the Remote - WSL extension.
  WSL folders are never scanned for markers.
- Windsurf, Zed and Sublime were left out of the first version.

## Not verified

Detected and iconized live on the development machine: VS Code, IntelliJ IDEA,
Android Studio and Visual Studio. Not verified: actually launching each IDE,
Cursor, Eclipse (including the unconfirmed manual-import note), the other
JetBrains products, and the WSL path.

## Open questions

- Eclipse: revisit if a supported command-line import appears.
- Offer Windsurf, Zed or Sublime Text if requested.
