# Contributor Guide

Stack: Go host (Wails/WebView2), React + xterm.js frontend, SQLite runtime
state, human-edited YAML session definitions, PowerShell 7 over ConPTY.
Windows only. The status of the work is in
[`21-engineering-handoff.md`](21-engineering-handoff.md).

## Prerequisites

- Go 1.26+ (this machine: `C:\Program Files\Go\bin`, not on PATH by default).
- Node.js + npm.
- WebView2 Evergreen runtime and PowerShell 7.
- Optional: `scripts/fetch-conpty.ps1` places `conpty.dll` + `OpenConsole.exe`
  next to the executable (bundled ConPTY); otherwise inbox ConPTY is used.

## Build, test, run

```powershell
cd frontend; npm ci; npm run build; cd ..      # writes cmd/taushell/frontend/dist
go vet ./...
go test -count=1 ./...
go build -tags production -ldflags "-H windowsgui" -o taushell.exe ./cmd/taushell
```

- Build the frontend first: Go embeds `cmd/taushell/frontend/dist`.
- The `production` tag is required, otherwise Wails builds a diagnostic stub.
- `taushell` opens the desktop app; `taushell list|start <id>|restore <id>`
  work without it.
- To include the app icon, run `python build\windows\gen_icon.py` before
  `go build` (requires Pillow). This generates
  `cmd/taushell/rsrc_windows_amd64.syso`, which Go links automatically
  (resource group ID 3, the one Wails loads for the window and taskbar).
  The generator is tracked; its `.syso`, `appicon.ico`, and `appicon.png`
  outputs are ignored. Builds without this step omit the custom icon.

## VS Code

`.vscode/` is ignored workspace-local configuration, not part of a fresh
checkout. Local configurations may provide:

- Launch: "TauShell: desktop app (debug)" (builds the frontend first, runs with
  `-tags=production`), plus CLI `list`, `start`, and `restore` (prompts for a
  session ID, runs in the integrated terminal).
- Tasks: frontend install/build, Go vet and test, and the default build task
  that produces `taushell.exe`.
- Settings: configure gopls with the `production` tag and add the installed Go
  directory to the terminal PATH if needed.

Debug runs do not use `-H windowsgui`, so a console window may appear. The
Go extension (Delve) is required.

## CI and releases

- `.github/workflows/ci.yml` runs on pushes to `main` and PRs: frontend build,
  token check and unit tests (`npm test`), `gofmt -l`, `go vet`, `go test`, and a
  release-build smoke test. `cmd/taushell/frontend/dist` is generated and
  gitignored, so the frontend must be built before any Go build or test.
- Releases: push a SemVer tag (`git tag v1.0.0 && git push origin v1.0.0`).
  `.github/workflows/release.yml` tests, builds
  `TauShell-<version>-windows-x64.zip` with `scripts/build-release.ps1`, writes
  `SHA256SUMS.txt`, attests build provenance and publishes a GitHub
  **pre-release**. Running the workflow manually with a version builds a
  **draft** instead (dry run; delete the draft afterwards).
- An explicitly authorized update of an existing release tag rebuilds that
  version and overwrites its ZIP and checksum assets. The release page and
  notes are preserved and the release remains a prerelease. Do not move an
  existing release tag without the owner's approval.
- Code signing is optional. When the repository secrets `AZURE_TENANT_ID`,
  `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SIGNING_ENDPOINT`,
  `AZURE_SIGNING_ACCOUNT` and `AZURE_SIGNING_PROFILE` (Azure Trusted Signing)
  exist, only `taushell.exe` is signed before packaging; the bundled
  `OpenConsole.exe` keeps Microsoft's signature. Without them releases are
  unsigned and the README documents the SmartScreen steps.
- The app version comes from `-X main.version`; local builds report `dev` and
  skip the update check.

## Rules of thumb

- Keep the core tool-independent; harness behavior lives in `internal/harness`.
- Do not log prompts, terminal bytes, env values, or credentials.
- Test sessions you create in your real profile show up on Home; delete them
  when done.
- Update the handoff and backlog when behavior changes; do not claim
  unverified behavior.
- Text files use LF line endings everywhere, including Windows working trees:
  `.gitattributes` (`* text=auto eol=lf`) overrides `core.autocrlf`, and
  `.editorconfig` tells editors. Scripts that write repository or release text
  files must write `\n` explicitly (for example
  `[IO.File]::WriteAllText(path, text)` with `` `n ``), not `Set-Content` or
  `Out-File`, which use CRLF on Windows. After pulling the `.gitattributes`
  change into an old CRLF checkout, refresh it once:
  `git rm --cached -r -q . && git reset --hard` (only with no local changes).
