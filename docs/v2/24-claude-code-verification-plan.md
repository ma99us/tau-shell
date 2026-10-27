# Claude Code Verification Plan

Status: **partly run (2026-10-27, Claude Code 2.1.311 on the dev PC).**
Everything that does not need a model reply was run headlessly through the real
session manager; see the results table in section 5. The login on this PC is a
Foundry key whose provider rejected every request ("User ... has been
deactivated"), so tests that need real model turns (recall after restore,
permission prompts, busy-state exit, spinner during a normal turn) are still
open. Earlier status: deferred by the owner (2026-10-24) until a PC with a
completed Claude Code login is available. Same areas are verified for GitHub Copilot CLI (see
[`21-engineering-handoff.md`](21-engineering-handoff.md)). Do not claim Claude
Code support beyond what this plan's results show. Record outcomes in the
results table at the end and in the handoff, stating plainly what failed.

## 1. What is and is not known today

Verified (any Claude Code version, no login):

- `claude --help` lists `--session-id`, `--resume` and `--model`, so the tool
  spec's capability probes pass (`internal/tools/tools.go`).
- Launch argv is `claude <launch_args> --session-id <id>`; restore is
  `claude <launch_args> --resume <id>` (`internal/harness/adapter.go`).
- Claude Code's first-run wizard (banner, theme menu, diff preview, "Select
  login method") renders correctly in the hosted terminal. Testing stopped
  there on purpose.
- Restoring a session that never got past first-run onboarding relaunches the
  wizard. That is the harness's own behavior, not a TauShell defect.
- Create dialog preview shows `--model sonnet` when `--model sonnet` is typed into the launch arguments.

Never verified (the gaps this plan closes):

| ID | Gap | Current code assumption |
|---|---|---|
| G1 | A completed OAuth/API login inside the hosted terminal was never done. | Login works through the browser handoff and persists. |
| G2 | Restore of a real conversation (at least one model turn) has never been run. `--resume <id>` is assumed to reopen the conversation started with `--session-id <id>`. | Adapter `ResumeArgs`. |
| G3 | State icon cue: braille spinner (U+2800-28FF) at the start of the terminal title = working, U+2733 = idle. Written from memory. | `frontend/src/attention.ts`, Claude branch. |
| G4 | No "needs input" cue. No Claude permission, trust-folder or question prompt has ever been captured. | None; falls back to output timing. |
| G5 | Graceful exit. Only Ctrl+C is sent; the "Graceful interrupt failed or timed out / force-terminated" notice is likely on every close. `/exit` or double Ctrl+C was never probed. | `gracefulExit` is empty for Claude in `harness.For`. |
| G6 | No "cannot resume yet" check. A session that never had a turn may show Restore, and the harness may error or restart onboarding. | `resumeCheck` is nil for Claude. |
| G7 | Default arg `--dangerously-skip-permissions` may show a one-time "Bypass Permissions" acceptance prompt or refuse to run in some setups (for example as administrator or outside a trusted folder). Never observed. | `defaultArgs` for Claude. |
| G8 | `--model` aliases and values (`sonnet`, `opus`, full names), and whether a model chosen at creation persists on restore. | Launch arguments in the Create dialog write `--model <value>` into `launch_args`. |
| G9 | Two installs exist on the dev PC (npm 2.1.133 first on PATH, winget 2.1.302). Behavior across both is unrecorded. Tested range is 2.1.133 to below 2.2.0. | `TestedFrom`/`TestedBefore` in the spec. |
| G10 | Claude TUI behaviors through ConPTY/xterm: alternate screen, mouse, bracketed paste, Shift+Enter or other modified keys, Ctrl+key combos, resize reflow, wide glyphs and emoji, long output scrollback. | Generic terminal fixture results only. |
| G11 | Session-ID reuse edge cases: `--session-id` with an ID Claude already used, `--resume` of an ID that does not exist, Start fresh after a real conversation. | `Manager.start`/`BeginFresh`. |
| G12 | Whether the first-run or per-folder trust prompt ("Do you trust the files in this folder?") appears for new project folders, and how the state icon looks while it waits. | Not handled; same as G4. |

## 2. Test machine prerequisites

Record all of these in the results first (they decide how to read the rest):

1. Windows version, PowerShell 7 version, WebView2 runtime version.
2. `where.exe claude` (every hit), `claude --version` for each, and which one
   TauShell shows on Home. Do not change PATH silently; test the second
   install by its explicit path only if you choose to cover G9.
3. A Claude account that can complete login (subscription or API key) and a
   throwaway project folder that is safe to modify (for example an empty
   `%TEMP%\claude-probe`). Never use a real repository for permission-prompt
   tests.
4. A TauShell build from this repository (`npm run build`, then
   `go build -tags production -ldflags "-H windowsgui" ./cmd/taushell`). Use a
   separate profile if the PC has real sessions. Delete every test session
   through the UI delete action afterwards.
5. A way to capture screenshots of the TauShell window only.

## 3. Optional temporary probe (for G3, G4, G12)

Titles and prompt text must be observed, not recalled. Temporarily add to
`TerminalPane` in `frontend/src/main.tsx`, after the terminal is created:

```ts
terminal.onTitleChange((title) => void app().LogFrontendFailure("title-probe", title, ""));
```

Titles then appear in the diagnostics log shown on Home (`taushell.log`).
Remove the probe before committing. For prompt text, rely on screenshots of
the window; do not paste prompt contents that include private data into docs.

## 4. Test plan

Run in this order; each step lists the gap it closes, the exact actions, and
the pass criteria. Mark any deviation as a finding, not a pass.

### T1: Login and first conversation (G1, G12)

1. Create a session: Claude Code, folder `%TEMP%\claude-probe`, default args.
   Note whether a first-run wizard, a trust-folder prompt, or a bypass
   acceptance prompt (G7) appears, and in which order. Screenshot each.
2. Complete login in the browser. Confirm the hosted terminal returns to the
   prompt and that closing/reopening TauShell does not ask to log in again.
3. Send one harmless prompt, for example "Reply with the single word ok".
   Confirm the reply renders and the session keeps working.

Pass: login completes and persists; every prompt in step 1 is recorded with
screenshots and its exact on-screen question text.

### T2: Restore a real conversation (G2, G11)

1. Continue the T1 session: send a prompt with a memorable fact ("Remember the
   word pineapple").
2. In TauShell, stop the session via its normal Stop/close path (note which
   notice appears, see T5).
3. Close TauShell fully, reopen it, and Restore the session.
4. Ask "What word did I ask you to remember?".

Pass: the same conversation reopens and answers correctly. Fail or partial:
record the exact harness message. If it silently starts a new conversation,
that is a defect: the UI must not present it as a restored conversation.
Also test: Start fresh after a real conversation (new `--session-id`, the old
conversation is not resumed) and a second Restore after a clean exit.

### T3: Restore of a session with no conversation (G6, G11)

1. Create a session, wait for the prompt (after login), send nothing, stop it.
2. Restore it. Record what Claude does: error text, wizard relaunch, or an
   empty new conversation. Capture the exit code and message.
3. Repeat after sending one prompt, and compare.

Outcome decides the fix: if Claude errors or loses state, add a Claude
`resumeCheck` (the Copilot one, `checkCopilotResume`, is the pattern) that
detects "no recorded conversation" and hides Restore in favor of Start fresh.
Inspect where Claude stores conversations (expected under
`%USERPROFILE%\.claude\projects\...`, **layout unverified**) before writing
the check, and make it fail open if the layout is not recognized.

### T4: State icons and titles (G3)

Enable the T3-section probe, then in one session observe, with screenshots
and the logged titles:

1. Idle at the prompt: title text and first character (expect U+2733?).
2. A multi-second task ("Count from 1 to 50 slowly, one number per line"):
   title while working (expect a braille spinner prefix?).
3. After completion: title returns to the idle form.
4. The sidebar icon and tab icon for each state: spinner while working, green
   dot when quiet, glow if the tab was not selected when it finished.

Pass: icons match reality without relying on the timing fallback. If the
titles differ from the assumption, update `readHarnessSignal` (Claude branch)
and the handoff, and note the Claude Code version the cue was verified on. If
Claude sets no useful title, add a status-row cue like Copilot's (last two
rows only; never whole-screen text) and record the real text.

### T5: Needs-input cue (G4, G12, G7)

Start a session **without** `--dangerously-skip-permissions` (clear the
Launch arguments in the Create dialog, as was done for Copilot). In the
throwaway folder trigger each, capturing screenshot plus last two rows:

1. Trust-folder prompt, if shown.
2. Permission prompt for a shell command ("Run the shell command: echo hi >
   out.txt").
3. Permission prompt for a file edit ("Create a file named a.txt").
4. A question or plan-approval prompt, if Claude offers one.

Record the title during each prompt (probe) and the last-two-row text. Add a
Claude `"input"` branch to `readHarnessSignal`, preferring the title if it
changes, else a footer regex like the Copilot one. Verify: orange square
appears (selected and unselected tab), glows while unseen, clears when
answered, and returns to working/quiet.

### T6: Graceful exit (G5)

On a session that has a real conversation, test each close path and record
elapsed time, exit code, and whether the force-terminate notice appears:

1. Current behavior: Stop and window-close confirm (Ctrl+C only).
2. Manually try `/exit` + Enter in the terminal: does Claude quit cleanly?
3. Manually try Ctrl+C twice: does it quit cleanly?
4. Then Restore each time (T2 criteria) to learn which path preserves
   resume.

If a path exits cleanly and restores, set Claude `gracefulExit` in
`harness.For` (see Copilot's steps as the pattern: Ctrl+C, pause, command,
Enter), add an adapter test, and re-run the shutdown smoke. If none beats
Ctrl+C, document that.

### T7: Defaults, model and launch args (G7, G8)

1. With defaults, confirm the bypass flag works: no permission prompts and
   no refusal message. Note any one-time acceptance dialog and any
   restrictions (administrator shell, non-trusted folder).
2. Create with launch arguments `--model` `sonnet`, then `opus`, then an invalid value. Check the
   preview, the YAML `launch_args`, the model Claude reports in its UI, and
   the error for an invalid value (is it visible in the terminal and does the
   session show as failed rather than silently running?).
3. Restore a session created with a model: confirm the same model is used.
4. Confirm reserved args are rejected in the Create dialog:
   `--session-id`, `--resume`.

### T8: Versions (G9)

For each Claude install on the PC (and at least one version below and above
the tested range if obtainable):

1. Home shows the version and "tested"/"untested" correctly; the capability
   probes still pass; no `missing:` warning.
2. Run T1-T2 on the install TauShell resolves from PATH.
3. If a newer or older version changes `--help` option names or behavior,
   update `Capabilities` and `TestedFrom`/`TestedBefore`, never silently pin
   or downgrade.

### T9: Terminal behavior with the real TUI (G10)

In one conversation, check and screenshot:

1. Typing, paste of multi-line text (bracketed paste), Backspace/arrows,
   Shift+Enter or the harness's newline key, Ctrl+C at the prompt, Esc.
2. Window resize and sidebar collapse/expand while Claude is idle and while it
   streams output; no garbled or duplicated redraw.
3. Unicode and emoji in prompt and reply (CJK, combining marks, emoji);
   box-drawing and colors; very long output and scrollback; mouse wheel.
4. Switch tabs away and back mid-stream; output continues and the screen is
   intact.
5. Keyboard focus returns to the terminal after creating and after selecting
   the tab.

Findings feed `docs/v2/15-terminal-rendering.md`; fix only clear TauShell
defects and report harness-side quirks as such.

## 5. Evidence and results

For each test record: date, TauShell commit, Claude Code version and install
path, Windows version, pass/fail, and the exact observed text for anything
unexpected. Copy this table into the handoff when complete:

| Test | Gap(s) | Result | Claude version | Notes / follow-up |
|---|---|---|---|---|
| T1 Login and first conversation | G1, G7, G12 | **Partial.** Trust-folder prompt captured. Its default selection is "No, exit", so Enter alone exits the harness with code 1 (Down then Enter accepts). No bypass-acceptance prompt appeared (the user's `skipDangerousModePermissionPrompt` setting is on). The login on this PC is a corporate Foundry key in `~/.claude/settings.json`, not OAuth, and the provider rejected every request (`401 Authentication Error ... has been deactivated`), so no model reply was ever produced and OAuth-in-terminal was not exercised. | 2.1.311 | Needs a working credential for real model turns. |
| T2 Restore real conversation | G2, G11 | **Partial.** `--resume <id>` reopens the transcript (earlier user prompts replayed, "Interrupted" notice), repeatedly, and Start fresh opened a clean session. Model recall ("pineapple") could not be checked because no reply was ever produced. `--session-id` of a used ID errors `Session ID ... is already in use.`; `--resume` of an unknown ID errors `No conversation found with session ID`. | 2.1.311 | Re-run recall check with a working credential. |
| T3 Restore with no conversation | G6, G11 | **Defect found, fixed.** A session that never got a first prompt has no transcript; Restore exited with `No conversation found with session ID` (exit 1). Added `checkClaudeResume` (transcript `projects/*/<id>.jsonl`, honors `CLAUDE_CONFIG_DIR`, fails open if the store is absent); verified live: Restore is now refused with "use Start fresh". | 2.1.311 | Transcript appears only after the first prompt. |
| T4 State icons and titles | G3 | **Assumption was wrong, fixed.** Idle title is `✳ Claude Code`; while working the prefix animates `◐`/`◑` (U+25D0-25D3), not braille; the trust prompt title is plain `claude`. `attention.ts` now treats U+25D0-25D3 as working. Only observed during provider retries, not a normal successful turn. | 2.1.311 | Re-check during a real turn. |
| T5 Needs-input cue | G4, G12 | **Partial.** Only the trust-folder prompt was reachable (footer `Enter to confirm · Esc to cancel`, plain title). Added a Claude `"input"` cue on that footer in the last two rows. Shell-command, file-edit and question prompts need model tool calls and were not captured. | 2.1.311 | Capture the permission prompts with a working credential. |
| T6 Graceful exit | G5 | **Fixed for idle sessions.** At an idle prompt `/exit` + Enter exits cleanly in about 0.4 s (Ctrl+C then `/exit` also works); Ctrl+C twice and Ctrl+D twice did not exit within 12 s. Claude `gracefulExit` is now Ctrl+C, `/exit`, Enter: idle Stop takes about 1.1 s with no force notice, previously about 3 s and always "force-terminated". While a request was retrying against the failing provider nothing exited it, so Stop still force-terminated after about 12 s (visible notice), and the next restore worked. | 2.1.311 | Busy-state exit unverified against a healthy provider. |
| T7 Defaults, model, args | G7, G8 | **Done except model persistence.** Defaults give `bypass permissions on` with no prompts. `--model sonnet` and `--model opus` show Sonnet 5.5 / Opus 5.5 in the header; an invalid model starts with a visible warning (`Model "..." isn't recognized`), not a failure. Reserved `--session-id`/`--resume` (also `--session-id=x`) are rejected. Model persistence on restore not checked. | 2.1.311 | |
| T8 Versions | G9 | **Partial.** Only one install now (npm 2.1.311, inside the tested range); capability probes and tool tests pass. No older or newer build tested. | 2.1.311 | |
| T9 Terminal behavior | G10 | **Partial, headless.** Through the real session manager and ConPTY (inbox backend), rendered with a VT emulator (pyte, not xterm.js): bracketed multi-line paste, Unicode/CJK/emoji/combining marks, and resize (80x24, 140x40, 100x30) reflowed cleanly; Esc and Ctrl+C at the prompt cleared input without exiting. Shift+Enter via CSI-u (`ESC[13;2u`) did not insert a newline. Not covered: the xterm.js window itself, mouse, scrollback, tab switching mid-stream, focus. | 2.1.311 | Needs a manual pass in the app window. |

## 6. Code changes this plan may trigger

Keep each small and tested; do not add them speculatively before the evidence
exists.

| Trigger | Change |
|---|---|
| T2 resume fails or loses state | Fix `ResumeArgs` or `Manager.start`; surface "could not resume" instead of implying success. |
| T3 outcome | Claude `resumeCheck` in `internal/harness/adapter.go` plus `resume_test.go` case. |
| T4/T5 | Claude branches in `frontend/src/attention.ts` (title first, last two rows second); update `docs/v2/21-engineering-handoff.md` state-icon text. |
| T6 | Claude `gracefulExit` steps; adapter test; shutdown smoke. |
| T7/T8 | Spec `Capabilities`, tested range, default args. |
| T9 | Terminal defects per `15-terminal-rendering.md`. |

When all tests have a recorded result, replace the "postponed" notes in the
handoff with the outcomes and remove this document's "open" status.
