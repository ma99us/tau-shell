# Attention Detection

Session activity is inferred from harness-owned cues where available; TauShell
does not compare whole-screen text or use cursor position to infer state.
Harness-specific parsing is in `frontend/src/attention.ts`.

## Signals

- **Claude Code:** terminal-title spinner glyphs indicate working, the idle
  title glyph indicates idle, and the trust-folder confirmation footer
  indicates input. The title glyphs and footer were observed on Claude Code
  2.1.311.
- **GitHub Copilot CLI:** busy/idle come from the harness's own session log
  (`activity: copilot-events`, `internal/harness/activity.go`): a user message,
  turn start or tool call means working; a turn end with nothing following for
  2 s means idle. This does not depend on the footer wording, which changes
  between modes (for example autopilot's `Autopilot · Allow All · Working ·
  286 B esc esc interrupt`). The screen cues remain the fallback, and the
  selection footer still indicates a prompt that needs user input (the log does
  not record it). Cues were observed on Copilot CLI 1.0.103 and autopilot on
  1.0.105. `combineSignal` merges the two.
- **Other tools or missing cues:** recent terminal output is used as a fallback.
  Output within 400 ms after a keystroke is treated as echo; output within the
  last 2.5 seconds indicates working. Without a harness cue, silence is not
  proof that the harness is waiting for input.

The signal reader checks only the harness title or specific status/footer rows,
not the full terminal buffer. A harness cue takes precedence over output timing,
with one exception: a working cue with no non-echo output for 15 seconds is
treated as stale (not working), because a harness that is still working keeps
redrawing its spinner or timer. The state rule is `nextActivity` in
`frontend/src/attention.ts`, covered by `frontend/test/attention.test.ts`.
This keeps provider output untouched and avoids false state changes from
unrelated screen redraws. Tools with reliable cues (Claude Code, Copilot CLI)
never use the output-timing fallback: when their cue is missing, the session
counts as not working, so idle redraws (promos, loading animations) cannot raise
repeated "done" alerts (`hasCueSignal`, `nextActivity`'s `cueTool` argument).

## User attention

A transition into a recognized input prompt can raise an input alert. A
transition from working to idle can raise a completion alert after at least
three seconds of work. Alerts are delivered only while TauShell is in the
background; selecting and viewing the session clears its unseen indicator.
Unknown or unsupported prompt types are not claimed as input-required.
