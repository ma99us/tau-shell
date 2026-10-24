# Recipe: Adding a Harness

This is a step-by-step recipe for adding support for another AI CLI harness.
Use Claude Code (`claude-code`) and GitHub Copilot CLI (`copilot-cli`) as the
worked examples; search the repository for those IDs to find every touch point
(see the checklist at the end). Architecture background is in
[`05-plugin-system.md`](05-plugin-system.md). Adapters are built in and
internal; there is no plugin loader.

Principles to keep:

- Core session management stays tool-independent. Harness-specific behavior
  lives in the adapter, the tool spec, and the frontend signal reader only.
- Launch and resume arguments are discrete argv values, never shell strings.
- Never claim a conversation was resumed unless the harness supports and
  confirms it. Unsupported behavior must be visible, not faked.
- Do not silently pin, downgrade, or auto-update the harness.

Throughout, `<id>` is the new harness ID (kebab-case, for example
`gemini-cli`).

## 0. Gather facts first

Run the real CLI and record, before writing code:

| Question | Used by |
|---|---|
| Executable name on PATH, and `--version` output format | tool spec, version parse |
| Help command (`--help`) and whether it lists the options below | capability probes |
| How to start a conversation with an ID TauShell chooses (Claude/Copilot: `--session-id <uuid>`) | `StartArgs` |
| How to resume by that ID (Claude/Copilot: `--resume <uuid>`) | `ResumeArgs` |
| Optional model option (`--model <name>`) | optional capability |
| Safe default args that avoid prompts blocking an unattended session (Claude: `--dangerously-skip-permissions`; Copilot: `--yolo`) | `DefaultLaunchArgs` |
| A clean quit input sequence that flushes state (Copilot: Ctrl+C then `/exit` Enter) | `GracefulExit` |
| Where the harness stores conversations, if you can tell "no conversation yet" from disk | `CheckResume` |
| Terminal title or status-row cues for working vs idle | frontend signal reader |
| winget package ID and official vendor install page | install source |

If the harness cannot start or resume with a caller-chosen ID, stop and
raise it: the current adapter shape (`StartArgs`/`ResumeArgs` keyed by a
TauShell-owned ID) assumes it can. Do not bend the core to fit one tool.

## 1. Register the ID in session config

File: `internal/session/config.go`

1. Add a constant next to `ClaudeCode` and `CopilotCLI` (type `Harness`).
2. Add it to the supported-tool check in `Config.Validate` (the
   `unsupported tool` error also lists supported tools; update the message).
3. Add an entry to the `reservedArgs` map in `ValidateLaunchArgs` listing the
   arguments TauShell manages for this harness (at least the start-ID and
   resume options). Users must not be able to override these through
   `launch_args`.

Tests: extend `internal/session/config_test.go` (validation accepts the new
tool and rejects its reserved args) and any table in `store_test.go` that
enumerates tools.

## 2. Declare the tool spec (detection, versions, capabilities, install)

File: `internal/tools/tools.go`

1. Add an `ID` constant next to `ClaudeCode`/`CopilotCLI`. The string must
   equal the session `Harness` value, because lookups cross the two packages
   (`tools.Lookup(tools.ID(harnessID))`).
2. Append a `Spec` to `specs` (display order). Fields:
   - `Executable`, `VersionArgs`, `HelpArgs`.
   - `TestedFrom` (inclusive) and `TestedBefore` (exclusive): the range you
     actually tested. Versions outside it produce a visible warning, not a
     block. Set `Minimum` only when older versions are known not to work.
   - `Capabilities`: one entry per option the adapter relies on, matched
     against the help text. Required ones (`--session-id`, `--resume` for the
     examples) make the tool unusable when missing. Mark features that only
     gate UI as `Optional: true` (for example `--model`): an absent optional
     capability never marks the tool unusable and never raises a warning.
   - `Install`: `WingetID` + `Source: "winget"` and the official `VendorURL`
     fallback. TauShell only installs after user confirmation.
3. Leave `Required` unset: harnesses never block Home (only PowerShell 7 and
   WebView2 are host prerequisites).

Tests: `internal/tools/tools_test.go` has table tests over `Specs()`; extend
the detection fixtures (fake executable output for version and help) so the
new spec is covered, including a missing-capability case.

## 3. Add the adapter

File: `internal/harness/adapter.go`

Add a `case session.<NewID>:` to `For`, returning a `cliAdapter` with:

- `executable`: the PATH name (`FindExecutable` uses `exec.LookPath`).
- `defaultArgs`: the editable template used to prefill new sessions
  (`DefaultLaunchArgs`). These are user-editable defaults, never identity
  arguments.
- `resumeCheck` (optional): a `func(harnessSessionID string) error` that
  returns a user-readable reason when the harness certainly cannot resume that
  ID, and nil when it can or cannot be determined. Fail open if the harness's
  storage is absent or its layout is undocumented. Copilot's
  `checkCopilotResume` is the pattern (and keeps a `...In(root, id)` variant
  so tests can use a temp directory).
- `gracefulExit` (optional): ordered `ExitStep{Input, Pause}` writes that make
  the harness quit by itself. Omit it when only a plain interrupt is known;
  shutdown then falls back to interrupt and, after a short wait, visible
  force-termination.

If the harness needs different start/resume argument shapes than
`--session-id <id>` / `--resume <id>`, do not special-case the manager:
give `cliAdapter` a per-harness `startArgs`/`resumeArgs` function field
(defaulting to the current behavior) and use it in `StartArgs`/`ResumeArgs`.
Those two methods are the only source of launch/resume argv; the manager,
the CLI `start`/`restore` commands, and the launch preview all call them.

Tests: `internal/harness/adapter_test.go` (default args, start/resume argv,
unknown harness error, graceful-exit steps) and `resume_test.go` if you add a
resume check.

## 4. Frontend wiring

Files: `frontend/src/bridge.ts`, `frontend/src/main.tsx`,
`frontend/src/attention.ts`

1. `bridge.ts`: add the ID to the three unions (`Session["tool"]`,
   `ToolStatus["id"]`, `SessionTemplate["tool"]`).
2. `main.tsx`: add the display name where it is hard-coded (sidebar subtitle,
   Home card icon letter, the "AI harness" `<select>`, the empty-state text).
   Prefer replacing the ternaries with a small `harnessLabels` lookup when you
   touch them. The Model field and launch preview are capability-driven and
   need no change: the field appears when the tool status reports an
   available `--model` capability.
3. `attention.ts`: add a branch in `readHarnessSignal(tool, terminal, title)`
   returning `"working"` or `"idle"` from tool-specific cues:
   - Claude Code reads the terminal title (spinner glyph vs idle glyph).
   - Copilot CLI reads only the last two buffer rows (status/footer text).
   - Never compare whole-screen text. Return `undefined` when there is no cue;
     the UI then falls back to output-silence timing, which is generic and
     already correct for new harnesses. Unverified cues must be documented as
     unverified in the handoff.

After editing run `cd frontend; npm run build` (rebuilds the embedded
`cmd/taushell/frontend/dist`).

## 5. Check the CLI and desktop paths

`cmd/taushell/desktop_windows.go` (`SessionTemplate`, launch preview,
unsupported-argument probe) and `main_windows.go` (`list`/`start`/`restore`)
resolve everything through `harness.For`, `tools.Lookup`, and the adapter. They
should need no change. If you find yourself editing them for a harness-specific
reason, move that behavior into the adapter instead.

## 6. Validate

1. `go vet ./...` and `go test ./...`.
2. Detection: launch the app; the harness row on Home shows the installed
   version, "tested"/"untested" support, and no `missing:` warning. Temporarily
   edit `TestedFrom`/`TestedBefore` to confirm the out-of-range warning path.
3. Create dialog: choose the harness; the launch arguments prefill with the
   defaults, the "Will run" preview shows `--session-id <new-session-id>`, and
   typing a reserved arg (`--resume`) is rejected. The Model field appears only
   if `--model` is detected.
4. Real session smoke test: create, start, type into it, quit it
   (graceful-exit path: confirm it exits without "force-terminated", or that
   the visible forced-termination notice appears if no exit steps exist),
   close and reopen TauShell, then Restore. Confirm a conversation with at
   least one turn actually resumes, and that a session with no turns shows
   Start fresh instead of a false Resume.
5. State icons: working shows the spinner, quiet shows the dot, and the unseen
   glow appears when the tab is not selected.
6. Delete throwaway sessions through the UI delete action and remove the
   test executable.

## 7. Documentation

- `docs/v2/05-plugin-system.md`: list the new built-in adapter.
- `docs/v2/21-engineering-handoff.md`: record tested versions, the signals
  used, and anything unverified (state it plainly).
- `docs/v2/18-yaml-schema.md`: add the new `tool` value.
- Update the Phase 1 scope text in `README.md`/`AGENTS.md` only if the
  owner has agreed to widen the supported-harness list; the current agreed
  Phase 1 set is Claude Code and GitHub Copilot CLI.

## Touch-point checklist

| Area | File |
|---|---|
| Harness ID, validation, reserved args | `internal/session/config.go` |
| Tool spec: version range, capabilities, install | `internal/tools/tools.go` |
| Adapter: executable, defaults, resume check, graceful exit | `internal/harness/adapter.go` |
| Types and labels | `frontend/src/bridge.ts`, `frontend/src/main.tsx` |
| Working/idle signals | `frontend/src/attention.ts` |
| Tests | `internal/session/config_test.go`, `internal/tools/tools_test.go`, `internal/harness/adapter_test.go` |
| Docs | `05-plugin-system.md`, `18-yaml-schema.md`, `21-engineering-handoff.md` |

Quick audit: `grep -rn "copilot-cli" --include=*.go --include=*.ts --include=*.tsx .`
(excluding `dist`) lists every place a harness ID is currently referenced.
