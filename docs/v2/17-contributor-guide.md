# Contributor Guide

Stack: Go host (Wails/WebView2), React + xterm.js frontend, SQLite runtime
state, human-edited YAML session definitions, PowerShell 7 (or an experimental
WSL2 distro, see [`docs/wsl/`](../wsl/)) over ConPTY.
Windows only. The status of the work is in
[`21-engineering-handoff.md`](21-engineering-handoff.md).

## Prerequisites

- Go 1.26+ (this machine: `C:\Program Files\Go\bin`, not on PATH by default).
- Node.js + npm.
- WebView2 Evergreen runtime and PowerShell 7 (optional for WSL-only use).
- Optional, for WSL work: WSL2 with a distro. Live WSL checks need a distro with
  the harness installed and signed in; none run in CI. Use disposable
  directories and sessions.
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
  work without it (`start` and `restore` reject WSL sessions).
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
  token check and unit tests (`npm test`), `gofmt -l`, `go vet`, `go test`, a
  release-build smoke test, and an end-to-end smoke test of the built ZIP
  (below). `cmd/taushell/frontend/dist` is generated and
  gitignored, so the frontend must be built before any Go build or test.
- End-to-end smoke test: `./scripts/smoke-test.ps1 -Zip <zip> -Version <ver>`
  (or `-Exe <taushell.exe>`). It starts the built app in an isolated profile
  (`USERPROFILE`, `TEMP`, `APPDATA`) with `TAUSHELL_SMOKE_OUT` set; the app
  then self-checks in its own WebView (`cmd/taushell/smoke_windows.go`) and
  `e2e/smoke.mjs` verifies: Home renders without an error banner, every method
  declared in `frontend/src/bridge.ts` is bound on the Go host, the log has no
  frontend failure or panic, and the version matches. It would have caught the
  release whose `GetRollbackVersion` bridge method was missing. WebView2 remote
  debugging (CDP) was tried first but does not open a port under Wails. Releases
  run the same test on the packaged ZIP before publishing. Add `-Lifecycle` to
  build a trusted disposable terminal fixture and exercise two launches: the
  first starts/stops a resumable session, then the second verifies auto-restore,
  one close confirmation, completed shutdown, and process exit. The fixture
  uses no AI provider or real user profile. CI and release smoke runs enable
  this lifecycle mode. Unit tests for the script: `node --test e2e/smoke.test.mjs`.
  Requires Node 22+.
- Lifecycle smoke gotchas: the fixture requires `TAUSHELL_FIXTURE_LOG` in its
  session environment; set it explicitly in the isolated session YAML rather
  than relying on a developer's inherited environment. The Home version is
  fetched asynchronously, so sample the visible version after Home renders.
  Also clear `TAUSHELL_LIVE` before `go test ./...` unless you intend to run
  the real, model-using harness probe.
- Real ConPTY tests must wait for a deterministic fixture-ready marker before
  sending terminal input. A fixed sleep can pass locally but race on hosted
  Windows CI: the process may exit before the write, causing `invalid argument`
  and invalid process/job handles during cleanup.
- Releases: push a SemVer tag (`git tag v1.0.0 && git push origin v1.0.0`).
  `.github/workflows/release.yml` tests, builds
  `TauShell-<version>-windows-x64.zip` with `scripts/build-release.ps1`, writes
  `SHA256SUMS.txt`, attests build provenance and publishes a GitHub
  **pre-release**. Running the workflow manually with a version builds a
  **draft** instead (dry run; delete the draft afterwards).
- Version bump timing: keep the default branch manifests at the last published
  app version. CI checks them before stamping its temporary `0.0.0-ci` smoke
  version. A tag release stamps the requested tag version before building and
  synchronizes the version-only manifest commit to the default branch only
  after publishing succeeds. A manual draft does not synchronize that commit;
  don’t commit CI smoke versions or pre-bump manifests by hand.
- After a tag release is published and CI succeeds, update the maintainer's
  local installation under `C:\Tools\TauShell\*` to that exact release. Download
  the published ZIP and `SHA256SUMS.txt`, verify the ZIP digest before staging
  it, and verify the installed app version afterward. Do not overwrite or stop
  TauShell while it has running sessions; wait for a confirmed shutdown first.
  The release also publishes `TauShell-Phone-<version>.apk`. If `adb devices`
  reports a connected device in `device` state, try `adb install -r` with that
  APK and verify the installed version. If the device is absent or Android
  rejects an in-place update (for example, due to a signing mismatch), leave
  the existing app and its data untouched and report the update as pending.
  These are post-publication deployment steps for the release task; GitHub's
  hosted workflow does not update a developer's local PC or connected phone.
- Before building, release and CI workflows set both frontend package manifests
  to the build version. After a successful tag release, the release workflow
  checks out the latest default branch separately and pushes a version-only
  commit for `frontend/package.json` and `frontend/package-lock.json`; rerunning
  an older tag cannot downgrade a newer version. A manual draft does not push a
  version commit. The workflow token must be allowed to push to the default
  branch; branch protection or permission failures make the workflow fail
  explicitly after publication. Commits pushed with `GITHUB_TOKEN` do not start
  another push-triggered CI run. Prerelease or build suffixes remain in the
  app and package versions; Windows fixed-version numbers and Android
  `versionCode` use the numeric release core.
- A local `scripts\build-release.ps1 -Version <version>` also updates those
  tracked frontend manifests before building. With `-SkipFrontend`, set the
  manifests using `node scripts\frontend-version.mjs set <version>` **before**
  building the frontend; the release script checks that they match, but cannot
  prove when prebuilt frontend assets were built. `-PackageOnly` packages the
  existing stage without changing manifests. The version helper's isolated
  tests run with `node --test scripts\frontend-version.test.mjs`.
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
- Check the three frontend manifest fields locally with
  `node scripts\app-version.mjs check`; compare them with the highest published
  GitHub release with `node scripts\app-version.mjs github` (requires `gh`;
  includes prereleases, excludes drafts, and paginates). CI runs the GitHub
  check before stamping `0.0.0-ci`, so a mismatch cannot be hidden by the smoke
  build. Tag/draft builds check their explicitly requested version instead,
  since it may not be published yet. Dependency, schema, protocol, fixture,
  and historical example versions are independent of the app release.
- Local Android builds derive `versionName` and `versionCode` from
  `frontend/package.json`; an explicit `-PappVersion` overrides the name,
  with a matching numeric code required if supplied. The code scheme is
  `major*10000 + minor*100 + patch` (minor and patch at most 99).
  Release CI verifies the generated APK metadata with
  `node scripts\app-version.mjs android <version> <output-metadata.json>`.
  Versioned E2E smoke tests also check Windows ProductVersion/FileVersion,
  the running bridge version, and the visible Home version.
- In-app update contract: each release must keep the assets`TauShell-<version>-windows-x64.zip` (flat, `taushell.exe` at the root) and `SHA256SUMS.txt`.
  Re-releasing the same tag is not offered as an update (strict version compare).

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
  files must write LF explicitly (for example via `WriteAllText` with a `` `n
  `` newline), not `Set-Content` or `Out-File`, which use CRLF on Windows.
  After pulling `.gitattributes` into an
  older checkout, do not reset or discard the worktree. Check `git status`,
  preserve local changes, and use a separate clean clone when you need to
  verify normalized line endings. Git applies the repository policy as files
  are checked out or rewritten.
