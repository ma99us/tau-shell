# Attention Detection

Session activity is inferred from harness-owned cues where available; TauShell
does not compare whole-screen text or use cursor position to infer state.
Harness-specific parsing is in `frontend/src/attention.ts`.

## Signals

- **Claude Code:** terminal-title spinner glyphs indicate working and the
  idle title glyph indicates idle. Recognized trust, permission, question, and
  plan-approval footers indicate input; the title alone does not. Working and
  input cues were recorded on Claude Code 2.1.316 through the session manager
  and ConPTY.
- **GitHub Copilot CLI:** busy/idle come from the harness's own session log
  (`activity: copilot-events`, `internal/harness/activity.go`): a user message,
  turn start or tool call means working; a turn end with nothing following for
  2 s means idle. This does not depend on the footer wording, which changes
  between modes (for example autopilot's `Autopilot · Allow All · Working ·
  286 B esc esc interrupt`). The screen cues remain the fallback, and the
  selection footer still indicates a prompt that needs user input (the log does
  not record it). Cues were observed on Copilot CLI 1.0.103 and autopilot on
  1.0.105. `combineSignal` merges the two.
- **GSD and Codex:** working/idle are read from their own session logs
  (`gsd-jsonl` and `codex-rollout`), rather than inferred from terminal output.
  Codex recognizes both legacy `task_started`/`task_complete` and newer
  `turn_started`/`turn_complete` rollout events. Reasoning deltas and active
  tool/command items also keep the session marked working when lifecycle
  events are delayed.
- **Tools without a cue or activity reader:** recent terminal output is the
  fallback. This currently includes OpenCode. Output within 400 ms after a
  keystroke is treated as echo; output within the last 2.5 seconds indicates
  working. Silence is not proof that the harness is waiting for input.

The screen reader checks only the harness title or specific status/footer rows,
not the full terminal buffer. A visible input cue wins over a fresh log signal;
otherwise a fresh log working/idle signal wins over the screen cue. Output
timing is used only when the harness declares neither screen cues nor an
activity reader. If either is declared but supplies no signal, the session
counts as not working rather than inferring work from redraws. For a harness
without an activity reader, a screen working cue with no non-echo output for
15 seconds is treated as stale. These rules are in `combineSignal` and
`nextActivity` in `frontend/src/attention.ts`, with focused tests in
`frontend/test/attention.test.ts`. Output-timing state is heuristic and is not
treated as a confirmed harness cue for automation.

## User attention

A transition into a recognized input prompt can raise an input alert. A
transition from working to idle can raise a completion alert after at least
three seconds of work. Alerts are delivered only while TauShell is in the
background; selecting and viewing the session clears its unseen indicator.
Unknown or unsupported prompt types are not claimed as input-required.
