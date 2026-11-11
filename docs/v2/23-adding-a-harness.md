# Adding a Harness

A harness is one AI CLI that TauShell launches, resumes and reads. Harnesses
are described by **declarative YAML manifests**, not Go code:

- The five built-in harnesses (Claude Code, GitHub Copilot CLI, OpenAI Codex
  CLI, OpenCode, GSD) are manifests embedded in the binary:
  `internal/harness/manifest/builtin/*.yaml`.
- You can add your own by dropping a `*.yaml` file into
  `~/.tau_shell/harnesses/`. It is loaded at startup (restart TauShell to apply
  edits) and appears on Home, in the New Session dialog and in the CLI.

Architecture background: [`05-plugin-system.md`](05-plugin-system.md).

Principles that still hold:

- Launch and resume arguments are discrete argv values, never shell strings.
- Never claim a conversation was resumed unless the harness supports and
  confirms it. A harness with no `resume_check` is shown as "resume cannot be
  verified".
- Do not silently pin, downgrade, or auto-update the harness.
- Reading a harness's private files is fail-open and fail-quiet.

## What a manifest can and cannot do

A manifest declares data: executable, version probes, capabilities, launch and
resume arguments, graceful-exit keystrokes, widgets, state cues (regular
expressions), skills and config-file locations, install sources and branding.

Anything that needs code, such as parsing a harness's private log or database,
is a **named reader** implemented in Go. A manifest only picks a reader by
name (a plain string such as `model: claude-jsonl`). The reader owns the
store's layout. User manifests can use any existing reader but cannot add new
ones; a new log format needs a Go change (see "Adding a reader").

| Field | Readers available |
| --- | --- |
| `resume_check` | `claude-transcript`, `copilot-session-state`, `gsd-session-file`, `codex-rollout`, `opencode-db` |
| `session.discovery` | `codex-rollout`, `opencode-db` |
| `model` | `claude-jsonl`, `copilot-events`, `gsd-jsonl`, `codex-rollout`, `opencode-db` |
| `title` | `claude-ai-title`, `copilot-workspace` |
| `activity` | `copilot-events`, `gsd-jsonl`, `codex-rollout` (working/idle from the harness's own log; preferred over screen working/idle cues) |
| `usage.reader` | `claude-jsonl`, `copilot-events`, `gsd-jsonl`, `codex-rollout`, `opencode-db` |
| `phone_answer` | `claude-permission`, `copilot-permission` |

Leaving a field out means the feature is absent for that harness. It never
becomes an error or a guess.

## Annotated example

```yaml
schema: 1                      # the only version this build understands
id: demo-cli                   # lowercase slug; built-in IDs are reserved
name: Demo CLI
short: Demo                    # used in tight spaces
executable: demo-cli           # bare name on PATH; no path, .exe or no extension
order: 100                     # optional; ties sort by id; user harnesses follow built-ins
probes:
  version_args: ["--version"]
  help_args: ["--help"]
  tested_from: "1.0.0"         # inclusive; outside the range is a warning, never a block
  tested_before: "2.0.0"       # exclusive
capabilities:                  # options matched against help text
  - { name: Resume a conversation, option: "--resume" }
  - { name: Select a model, option: "--model", optional: true }
launch:
  default_args: ["--fast"]     # editable preset for new sessions
  reserved_args: ["--resume", "--session-id"]   # managed by TauShell; rejected in launch_args
session:
  id: tau                      # tau = TauShell picks the ID; discover = the harness picks it
  start_args: ["--session-id", "{id}"]
  resume_args: ["--resume", "{id}"]
  # file: "~/.demo/{id}.jsonl" # optional; {session_file} expands to it and its folder is created
  # resume_position: before    # put resume args before launch_args (default after)
graceful_exit:                 # ordered writes that make the harness quit by itself
  - { input: "/exit\r", pause_ms: 200 }
widgets:                       # quick actions typed into the session (unconfirmed by the harness)
  commands:
    - { label: Compact conversation, text: "/compact" }
  # model_picker: "/models"  # for harnesses without "/model <id>": open this picker, then type
  #                           # each models entry into its search box and press Enter
  interrupt_keys: ["\x1b"]
cues:                          # ordered rules; the first match wins
  - { state: working, source: last-rows, rows: 2, pattern: 'esc to interrupt', ignore_case: true }
  - { state: idle,    source: title,     pattern: '^Demo' }
install:
  vendor_url: https://example.com/demo   # https only; opened by TauShell
brand:
  mark: monogram               # user harnesses use the monogram mark
  text: Dm                     # up to 2 letters
  color: "#3aa675"
```

`{id}` is the harness conversation ID and `{session_file}` is the expanded
`session.file`. Path templates also accept `~` and `{env:NAME|fallback}`.

### Cue sources

| Source | Reads |
| --- | --- |
| `title` | the terminal title |
| `last-rows` (with `rows: N`) | the last N screen rows, including empty ones |
| `last-text-rows` (with `rows: N`) | the last N non-empty rows within the bottom 6 |

Patterns use the common subset of Go RE2 and JavaScript: no inline flags,
named groups, lookaround or `\A \z \Z \x{...} [[:class:]] \Q \E`. Write
non-ASCII as `\uXXXX`. Use `ignore_case: true` for case-insensitive matching.
Never match against the whole screen.

## Trust and safety for user manifests

A user manifest decides what program TauShell starts, so it is treated as
untrusted input:

- Strict parsing: unknown fields are errors; the file must be one document and
  at most 64 KB. A bad file is shown on Home and skipped; it never stops
  TauShell from starting.
- `executable` must be a bare name resolved on PATH. Home shows the resolved
  path.
- A user manifest cannot start sessions until you press **Confirm** on Home.
  The confirmation is stored in `~/.tau_shell/harness-trust.json` against the
  manifest's SHA-256, so editing the file asks again. The check is enforced in
  the launch path, so the CLI `start` and `restore` commands are covered too.
- User manifests may only declare a `vendor_url` (https). One-click winget/npm
  installs exist for built-ins only.
- Do not put secrets in a manifest.

## Steps

1. Run the real CLI and record: executable name, `--version`, `--help`
   options, how to start and resume with a TauShell-chosen ID (or whether the
   harness picks its own), safe default args, a clean quit sequence, and any
   terminal title or footer text that distinguishes working, idle and
   waiting-for-answer.
2. Write `~/.tau_shell/harnesses/<id>.yaml` (or, for a built-in, add a file to
   `internal/harness/manifest/builtin/`).
3. Restart TauShell. Check Home: the harness shows its resolved path and any
   load error. Confirm it.
4. Create a session with it. The "Will run" preview shows the argv; reserved
   arguments are rejected.
5. Run a real session, quit it, restart and restore. Confirm a conversation
   with at least one turn resumes, and that a harness without a resume reader
   is labeled unverified rather than claimed resumed.

## Adding a built-in harness

1. Add `internal/harness/manifest/builtin/<id>.yaml`.
2. Add a constant in `internal/session/config.go` and `internal/tools/tools.go`
   only if Go code refers to the ID directly (the lists themselves come from
   the manifests).
3. Use the vendor mark or the monogram, and keep the brand color as a
   `--brand-*` token in `frontend/src/theme/` for built-ins.
4. Regenerate fixtures: `UPDATE_GOLDEN=1 go test ./internal/tools ./internal/harness/manifest`.
   `internal/tools/testdata/harness_specs.golden.json` freezes the generated
   tool specs and `frontend/test/fixtures/harnesses.json` is what the frontend
   cue tests load; both must match the manifests.
5. Extend `internal/harness/characterization_test.go` with the new harness's
   start, resume, discover and exit behavior.
6. Run `go vet ./...`, `go test ./...`, and in `frontend/`: `npx tsc --noEmit`,
   `npm test`, `npm run build`.

## Adding a reader

Readers are the only per-harness Go code. Each is selected by name:

| Kind | Where to add it |
| --- | --- |
| Resume check, discovery | `internal/harness/adapter.go` (`resumeCheckers`, `discoveryReaders`) |
| Model | `internal/harness/model.go` `CurrentModel` |
| Title | `internal/harness/titles.go` `ConversationTitle` |
| Usage | `internal/usage/parse.go`, `parse_harnesses.go`, `ledger.go`, and `internal/sessionhost/usage_windows.go` (OpenCode DB ingest) |
| Phone answer | `frontend/src/attention.ts` `answerPlan` (`internal/control` only reads the manifest field) |

Then add the name to the matching set in `internal/harness/manifest/validate.go`
so manifests can select it. Readers must fail open, never fabricate cost, and
dedupe records.

## Touch-point summary

| Area | Where |
| --- | --- |
| Manifest schema, validation, loading | `internal/harness/manifest/` |
| Built-in harnesses | `internal/harness/manifest/builtin/*.yaml` |
| Launch/resume argv, discovery, graceful exit | `internal/harness/adapter.go` (generic, reads the manifest) |
| Tool specs (detection, install) | `internal/tools/tools.go` `harnessSpecs()` (generated) |
| Supported IDs, reserved args | `internal/session/config.go` |
| Trust and launch gate | `internal/harness/manifest/trust.go`, `manifest.Allowed` |
| Host binding for the UI | `cmd/taushell/harnesses_windows.go` (`GetHarnesses`, `ConfirmHarness`) |
| Cue engine, answer plans | `frontend/src/attention.ts` |
| Registry, icons, user-harness panel | `frontend/src/harnesses.ts`, `harnessIcons.tsx`, `userHarnesses.tsx` |

Known limits: manifests load at startup only; user manifests cannot supply new
log parsers, vendor marks or one-click installs.

WSL: a harness whose adapter returns `DiscoversSessionID()` true (resume ID found by reading a Windows folder) is rejected for WSL sessions, and readers must take the per-session home (`harness.*In`) instead of `os.UserHomeDir()` to work inside a distro. Declare no Windows-only paths in a manifest if the harness should work in WSL. See [`docs/wsl/06-backlog.md`](../wsl/06-backlog.md) for per-harness WSL gaps.
