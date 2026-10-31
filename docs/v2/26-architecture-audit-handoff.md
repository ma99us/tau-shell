# Architecture audit handoff

**Audit date:** 2026-10-31. **Status:** Open findings from a read-only
architecture-cleanup dry run.

This is the continuation handoff for
[`architecture-cleanup-playbook.md`](../architecture-cleanup-playbook.md).
The audit did not implement its recommendations. This document and its link
from the [engineering handoff](21-engineering-handoff.md) are documentation
only. Recheck the current code, tests, and workspace before acting: file
locations and conclusions below describe the audit snapshot, not a guarantee
about a later checkout. Follow the user's current instructions on what to
implement.

The audit found targeted reliability and consistency improvements, not a need
for a broad rewrite. Preserve existing behavior unless a change is explicitly
agreed, and keep each pass small enough to test and review.

## Starting context and safeguards

- The audited checkout was the **main workspace** at `C:\Projects\tau-shell`,
  on `main` at commit `0d7768079270733489a889cdbd8686e69475ec9f`. It was
  already dirty; the worktree list showed that checkout as the only worktree.
  Treat this as a historical snapshot and run `git status` before future work.
- Earlier, uncommitted work includes LF policy (`.gitattributes` and
  `.editorconfig`), the shared `internal\fsutil` replacement helper, and
  splitting `frontend\src\main.tsx` into `createSession.tsx`, `dialogs.tsx`,
  `errors.ts`, `home.tsx`, `sessionVisuals.tsx`, and `terminalPane.tsx`. It
  also includes frontend attention tests, CI/release and script updates,
  documentation updates, and changes to Go callers. Some of these files are
  untracked. Do not discard them, clean the checkout, or assume a clean diff.
  Nothing was staged, committed, or pushed as part of this handoff.
- The frontend entry point was reconstructed from HEAD plus its earlier
  uncommitted edits before the split. The resulting files were checked, but a
  live UI smoke was not repeated after the split. Preserve the current work
  rather than reconstructing it again.
- Follow `AGENTS.md`, including the private-path restrictions, LF-only file
  policy, Phase 1 scope, and constraints on Git operations. Do not run the
  destructive checkout-refresh command currently shown in the contributor
  guide against this or any other dirty checkout.
- Phase 1 remains a local Windows GUI with PowerShell 7/ConPTY, human-edited
  YAML intent, separate runtime state, and harness-specific adapters. Keep
  `list`, `start`, and `restore`; do not add a second TUI, remote access,
  mobile features, or a web dashboard. Local flash/toast alerts are in scope.
- Preserve session-scoped environment handling and never log secrets,
  terminal contents, prompt text, or argument values. Do not rewrite provider
  output. Keep background xterm instances alive and receiving output when a
  live session's tab is closed to the background.
- Restore the PowerShell process and resume the harness conversation as
  separate outcomes. Do not report a conversation as restored merely because
  a process started. Do not delete the user's working directory or a
  harness's own conversation data when deleting a TauShell session.
- Claude Code model/login-dependent verification is **postponed for another
  PC**; do not make it a prerequisite for independent work. The detailed
  gaps and planned live tests are in
  [`24-claude-code-verification-plan.md`](24-claude-code-verification-plan.md).
  GSD and OpenCode launch, resume-ID recording, graceful stop, and restore
  were previously verified live. Model replies and usage ingestion were not;
  Codex live use was not verified. Do not expand these claims without evidence.

## Priority and dependencies

Impact and refactor risk are estimates, not evidence that a defect has
occurred in a live session. Correct the low-risk documentation and script
hazards when convenient, then prioritize the data-integrity and responsiveness
items. Shared lifecycle refactoring should incorporate the identity fix rather
than creating a second claim implementation.

| ID | Open work | Impact / risk |
|---|---|---|
| A1 | Concurrent resume-ID claims and launch correlation | High / medium-high |
| A2 | Skill preference load/save consistency | High / medium |
| A3 | YAML edits and multi-store session deletion | High / medium |
| A4 | Global manager lock during launch and probing | High / medium |
| A5 | Duplicated and divergent CLI/desktop lifecycle policy | Medium / medium |
| A6 | Documentation drift and unsafe checkout guidance | Medium / low |
| A7 | Frontend clone lifecycle, error reporting, and coverage | Medium / low-medium |
| A8 | Replacement guarantees and binary-safe LF synchronization | Medium / low-medium |
| A9 | Smaller adapter and performance opportunities | Low / case-dependent |

## A1. Resume-ID claims and launch correlation

**Evidence.** `internal\sessionhost\discover_windows.go` first calls
`HarnessSessionIDs` and later calls `SetHarnessSessionID`. The latter, in
`internal\session\store.go`, only prevents overwriting an ID already stored
on the *same* TauShell session. The `session_runtime` schema has no uniqueness
constraint for a harness and nonempty conversation ID. The CLI has the same
separate lookup and claim in `cmd\taushell\main_windows.go`.

An in-memory SQLite check accepted the same harness conversation ID for two
different session rows. This establishes that the store permits duplicate
claims; **a concurrent live harness race was not reproduced**. Matching by
working directory and launch time can also be ambiguous or select the wrong
conversation; an atomic claim alone does not establish correct correlation.

**Direction.** Make reservation of a nonempty ID atomic across store handles
and TauShell processes, with a clear conflict outcome. Correlate discovery to
the current launch; when a confident match is unavailable, leave the ID
unclaimed and surface the ambiguity rather than guessing. Account for
existing databases that may already contain duplicate IDs: do not silently
discard or reassign a user's saved conversation during migration.

**Completion checks:**

- Deterministically race two candidates for the same harness/ID using separate
  store handles against one temporary database; at most one claim succeeds.
  Test empty IDs, different harnesses, reclaims by the same session, and
  legacy duplicate state or migration behavior.
- Exercise simultaneous launches in the same folder, ambiguous discoveries,
  external conversations, a stale discovery worker after restart/fresh start,
  and persistence failures. No old launch may claim an ID for a newer launch.
- Preserve existing IDs on failed restore. Both CLI and desktop report
  ambiguity/conflicts and do not claim successful conversation restoration
  without adequate evidence.
- Extend `internal\session\store_test.go`,
  `internal\sessionhost\manager_windows_test.go`, and relevant harness/CLI
  tests; use temporary fixtures rather than real conversations.

## A2. Skill preference state consistency

**Evidence.** `internal\skills\library.go` ignores read and JSON errors in
`loadState`. Missing state on first run is normal, but unreadable or malformed
state can silently turn previously disabled skills back on. `SetEnabled`
mutates `l.state.Disabled` before saving, and `Apply` changes
`l.state.Applied` before saving. A failed write can therefore leave backend
memory changed while the UI rolls back.

**Direction.** Distinguish an absent first-run file from a read or decode
failure, surface the latter, and only publish a new in-memory preference or
active snapshot after its persistence succeeds. Preserve the last known
valid state; do not overwrite malformed user data merely to recover.

**Completion checks:**

- Test first run, valid persisted settings, malformed JSON, unreadable state,
  and error visibility without exposing skill contents or secrets.
- Inject failures during `SetEnabled` and `Apply`; the reported result,
  in-memory state, persisted state, UI rollback, and a subsequent library
  reopen must agree. Include successful retry and concurrent calls.
- Extend `internal\skills\skills_test.go`; ensure failed snapshot activation
  does not silently select a different skill set.

## A3. YAML edits and session deletion

**Evidence.** `session.UpdateConfigField` in `internal\session\config.go`
reads and rewrites YAML without coordinating concurrent in-app or external
edits. `setMappingString` replaces the value node, which can discard comments
attached to that node. The current comment test covers a document-level
comment, not all affected placements.

`Manager.Delete` in `internal\sessionhost\manager_windows.go` removes YAML
before deleting runtime state. `Store.Delete` in `internal\session\store.go`
then deletes runtime and ordering rows in separate SQL statements. Failures
can leave a partial deletion that is difficult to retry.

**Direction.** Preserve human edits and value/key comments, serialize
TauShell-originated updates, and detect or safely handle external edit
conflicts. Specify the actual cross-process guarantee; a check followed by a
rename alone is not a general atomic compare-and-swap. Use a recoverable
deletion protocol and a database transaction where appropriate. A database
transaction by itself cannot make filesystem and database changes atomic.

**Completion checks:**

- Race two in-app field updates and simulate an external edit between read and
  replacement. Both edits survive or the losing operation returns an explicit,
  actionable conflict; no silent overwrite. Test inline, leading, and
  trailing comments, plus unrelated keys and supported field removal.
- Inject failure at each deletion stage and simulate restart/retry. Runtime
  and ordering rows must not be partially removed, and the session must be
  recoverable or deletion retryable without hiding the outcome.
- Retain the running/starting session guard, usage-history policy, and
  prohibition on deleting project or harness data. Extend
  `internal\session\config_test.go`, `store_test.go`, and the manager deletion
  tests.

## A4. Launch lock blocks unrelated session operations

**Evidence.** `Manager.start` holds `manager.mu` across filesystem and
database operations, skill preparation, a capability probe, and process
startup in `internal\sessionhost\manager_windows.go`. The
`SupportsPluginDir` help probe in `internal\harness\skills.go` has a
20-second timeout. `manager.get`, used by terminal input, resize, and stop,
also takes `manager.mu`, so a slow start can block unrelated sessions.

**Direction.** Restrict the global lock to shared manager state and use a
per-session reservation or equivalent coordination for long-running startup.
Retain same-session and cross-process launch guards, shutdown coordination,
process cleanup, event ordering, and existing lifecycle semantics.

**Completion checks:**

- Block a fake launch/probe on a channel and verify an already-running
  session can write, resize, and stop before that launch is released.
- Race starts of the same ID, start against delete/close, and process-start or
  `MarkRunning` failures. No duplicate child, leaked process, deadlock, or
  lost error is acceptable. Also test independent concurrent launches.
- Extend `internal\sessionhost\manager_windows_test.go` using fake processes
  and deterministic synchronization rather than sleeping for 20 seconds.

## A5. Share lifecycle policy without hiding CLI failures

**Evidence.** Recovery logic and its one-minute startup delay are duplicated
in `cmd\taushell\main_windows.go` and
`internal\sessionhost\manager_windows.go`. The CLI discovery loop ignores
lookup, ambiguity, and persistence errors; the desktop path logs them and
records an ambiguity note. Desktop restore calls `Adapter.CheckResume`, while
the CLI restore path does not.

**Direction.** Extract the shared recovery and, where useful, discovery/claim
policy into a tool-independent owner. Keep presentation and logging appropriate
to each interface. Resolve A1's identity semantics in both paths; do not
silently convert unsupported or failed resume checks into success.

**Completion checks:**

- Cover recent/abandoned starting records, live/dead processes, repeated
  concurrent state changes, and preservation of saved resume IDs in both
  entry paths.
- Verify CLI and desktop handle discovery errors, ambiguous IDs, claim
  conflicts, and unsupported/failed resume checks explicitly. Preserve
  `list`, `start`, and `restore` behavior and adapter-specific argv.

## A6. Correct documentation and workflow drift

The following are observations to reconcile against current code and settled
product decisions, not a request to change product behavior just to match
stale prose:

- `23-adding-a-harness.md` still says to stop when a harness chooses its own
  ID, although Codex/OpenCode support discovered IDs. Its frontend
  registration instructions also predate the component split and harness
  registry. Document the supported alternatives and their verification limits.
- `01-system-architecture.md` says setup/version probes and confirmed installs
  are unimplemented, excludes notifications from Phase 1, and says startup
  never auto-resumes. Reconcile those statements with the implementation and
  current Phase 1 decisions.
- `22-local-attention-notifications.md` says no PTY-silence heuristic, while
  `nextActivity` in `frontend\src\attention.ts` uses output timing when no
  harness cue is available and its result feeds `attentionEdge`. This also
  touches a documented owner decision: determine whether the code or the
  prose is wrong before changing behavior; do not silently alter alert rules.
- `21-engineering-handoff.md` places `StateIcon` in `main.tsx`; it now lives
  in `sessionVisuals.tsx`. Clarify that attention helper tests exist, while
  component/widget lifecycle coverage is still missing.
- `17-contributor-guide.md` suggests `git rm --cached` followed by
  `git reset --hard` for refreshing an old checkout. Replace or clearly guard
  this dangerous guidance with a non-destructive approach; never execute it on
  the current dirty checkout.

**Completion checks:** trace each changed statement to code or an explicit
owner decision, retain live-versus-unit-test qualifications, and check links
and the LF policy. Document unresolved behavior decisions rather than
inventing a new requirement.

## A7. Frontend clone lifecycle and error handling

**Evidence.** In `frontend\src\main.tsx`, `duplicateSession` invokes
`void launch(copy, "start")` and then clears `busy` in its `finally`, while
the asynchronous launch is still in progress. `home.tsx` and
`terminalPane.tsx` also turn some usage, quota, branch, or model failures
into empty values without explaining the error.

**Direction.** Keep the clone operation busy through launch and provide
appropriate, non-spammy error visibility for failed background reads. A
genuinely unavailable/unsupported value must remain distinguishable from a
failed request. Preserve session ordering, mounted background terminals,
terminal ownership, and existing UI conventions.

**Completion checks:**

- With a deferred launch promise, verify controls stay busy until launch
  completes; test duplicate failure, launch failure, and subsequent retry.
- Test repeated polling failures and recovery without flooding the user or
  logs; distinguish failure from an intentionally empty or unsupported value.
- Add component/lifecycle coverage using the existing tooling where practical,
  and perform a Windows GUI smoke after the split: launch, clone, tabs,
  background output/reopen, stop, and failure notices. The current eight
  frontend tests cover attention helpers, not these flows.

## A8. File replacement and binary-safe LF synchronization

**Evidence.** `internal\fsutil\fsutil.go` writes and syncs a temporary file
beside the target, then calls `os.Rename`. This reduces in-place truncation
risk, but Go does **not guarantee that `os.Rename` is atomic on Windows**;
the helper and `session\config.go` comments currently overstate crash
guarantees. Temp cleanup errors are ignored, and existing tests cover success
and a missing parent, not failures after temporary-file creation.

`scripts\sync-cookbook-skills.ps1` treats every NUL-free file as UTF-8 text
and normalizes CRLF. A non-UTF-8 binary without NUL can be corrupted; an
in-memory example changed bytes `FF 0D 0A` to `EF BF BD 0A`. `Get-Digest`
also decodes files rather than hashing the original bytes. All 25 bundled
files inspected in the audit were valid UTF-8 with no NUL, so no current
affected binary was identified.

**Direction.** State only the replacement and crash guarantees actually
established on Windows; surface relevant cleanup failures and preserve the
original file on failed replacement where possible. Normalize only files
known to be text, using strict decoding and an explicit classification policy;
valid UTF-8 without NUL alone is not proof that a file is text. Hash binary
files as bytes and ensure check mode follows the same normalization policy.

**Completion checks:**

- On Windows, inject write, sync, close, rename, and cleanup failures after
  temporary-file creation. Verify error reporting, original-file contents,
  and temp-file handling. Document any remaining OS/crash limitations rather
  than asserting unsupported atomicity.
- Use disposable cookbook fixtures to test UTF-8 text with CRLF, emoji and
  BOM as applicable; invalid UTF-8 without NUL; binary with NUL; and a
  NUL-free, valid-UTF-8 binary file. Binary bytes must remain unchanged and
  check mode must detect actual byte differences without modifying either
  source or bundled files.

## A9. Smaller opportunities

- Harness skill eligibility is hard-coded in `Manager.skillArgs`, OpenCode
  usage ingestion is special-cased in
  `internal\sessionhost\usage_windows.go`, and harness IDs are repeated across
  registries. Consider adapter-owned capabilities and registry contract
  tests only if they simplify extension without enabling unverified support.
  GSD/OpenCode per-launch skill routes are not verified.
- `checkCodexResumeIn` in `internal\harness\gsd_codex.go` reads an entire
  transcript to check for more than one line. A streaming check could reduce
  memory use; preserve existing empty/one-line and error semantics and test
  long lines before changing it.
- `frontend\src\bridge.ts` exports a seemingly unused `events` helper. Verify
  all references, including indirect consumers, before removing it. Do not
  perform low-value cleanup merely to reduce line count.

## Validation baseline and next-pass checklist

The read-only audit ran the existing frontend tests (8 passed), the theme-token
check, and focused Go session/harness/fsutil tests successfully. The first
`npm test` was run from the repository root and failed because there was no
root `package.json`; it passed from `frontend`. A first-party frontend import
scan found no runtime cycles. A previous implementation pass also passed the
frontend build, `gofmt -l`, `go vet ./...`, and `go test -count=1 ./...`.

The current Windows Go dependency graph resolved offline, but
`go mod tidy -diff` was **inconclusive** with networking disabled because
some transitive test dependencies were not cached. This is not evidence that
the module is tidy or that it is broken. No real harness race, live harness
probe, or post-split live GUI smoke was performed during the dry run.

For each later implementation pass:

1. Recheck the dirty workspace and the relevant findings; read the applicable
   V2 specs and existing tests. Confirm any decision that would materially
   change behavior, scope, or deletion of user data.
2. Add focused regression and failure-path tests for the chosen item. Run
   frontend commands from `frontend`; use disposable directories/databases for
   tests. Build the frontend before Go commands that need embedded assets.
3. Run the relevant focused tests, then the full checks in the engineering
   handoff and contributor guide as appropriate. Run the Windows GUI/ConPTY
   smoke for significant user-visible or runtime changes; report unavailable
   checks rather than substituting a compile-only claim.
4. Do not install dependencies unless manifests changed or a required command
   reports missing dependencies. Stop a running `taushell.exe` by its specific
   PID before rebuilding it; do not terminate processes by name.
5. Update the affected V2 docs and this handoff with completed work, exact
   validation, and remaining gaps. Never commit or push without authorization.

The local Wails fork has a documented navigation callback patch in
`third_party\wails\TAUSHELL_PATCHES.md`; do not remove it merely to reduce
dependencies. Built executables and embedded frontend output are gitignored
build products, not a replacement for the source and untracked files above.
