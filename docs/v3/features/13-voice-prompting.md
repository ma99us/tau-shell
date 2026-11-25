# 13 - Voice prompting

Status: **future planning**. Unranked. Source: user request (2026-11-23). Nothing here is built or tested.

## Problem

Prompts for a harness are often long. Dictating them would be faster, but TauShell has no
microphone or speech code.

## User stories

- As a user, I want to speak a prompt and have the text appear in the harness prompt, so
  that I do not have to type long prompts.
- As a user, I want to review the text before it is sent, so that a misheard word never runs
  as a command.
- As a user, I want audio to stay on my machine unless I explicitly choose otherwise.

## Stage 0 - Windows Voice typing (try first, no code)

Windows Voice typing (Win+H) types into the focused control. It may work in the xterm.js
terminal and prompt box as it is. To check on a real window:

1. Focus a session terminal and press Win+H; dictate a sentence.
2. Check that text arrives in the harness prompt, that punctuation is sane and that it does
   not press Enter.
3. Repeat in the queue prompt editor and the preset editor.
4. Note: it needs Windows 10 2004 or later (11 is better), an enabled microphone, and may need
   online speech recognition turned on in Windows privacy settings (audio then goes to
   Microsoft; say so in any docs).

Known risks: xterm.js uses a hidden textarea, so composition or insertion may be dropped or
doubled; the harness may treat pasted-like input as a draft or a submit; focus can move off
the terminal when the Voice typing panel opens. If it works, document it as a tip. If it does
not, move to Stage 1. TauShell cannot control it, so no cue or guard applies to its input.

Result: Voice typing works in the terminal (tried by the user, 2026-11-23). TauShell now has a
mic button at the bottom of the session side rail that focuses the terminal and sends Win+H
(`cmd/taushell/voice_windows.go`, `frontend/src/voiceButton.tsx`). Windows draws the panel, so
it cannot be hidden, and the pulsing state only shows what TauShell asked for, not that Windows is
still listening. Not yet checked in a real TauShell window.

## Stage 1 - Local Whisper (if Stage 0 is not good enough)

Recommended engine: **whisper.cpp** (`whisper-cli.exe`, MIT), run as a child process.

| Option | License | Notes |
|---|---|---|
| whisper.cpp | MIT | Prebuilt Windows zips (CPU x64, BLAS, CUDA). Models are separate downloads, about 75 MB (tiny) to 1.5 GB. Best accuracy for technical words. |
| faster-whisper | MIT | Needs Python and CTranslate2; heavier to ship. |
| sherpa-onnx | Apache-2.0 | Streaming (live words); more setup. |
| Vosk | Apache-2.0 | Small and streaming; less accurate. |
| Cloud (Azure, OpenAI) | n/a | Audio leaves the machine; explicit opt-in only. |

Design:

1. **Capture.** The frontend records with `getUserMedia` and an `AudioWorklet` at 16 kHz mono
   and sends a WAV to the Go host through a bridge call. Check first that WebView2 grants the
   microphone permission (the bundled Wails layer falls back to the WebView2 default; TauShell
   sets no policy yet).
2. **Transcribe.** Go runs `whisper-cli` on a temp WAV and deletes it at once. Never log audio
   or transcript text.
3. **Result.** The transcript goes into the prompt composer or queue as a draft. It is never
   auto-sent; the existing guards (idle only, never into a permission prompt, never over an
   unsent draft) still apply.
4. **UI.** A mic button in the session toolbar (push-to-talk or toggle, plus a hotkey), a
   visible recording state, a transcribing state and a clear error. Capability-gated and off
   until enabled.
5. **Setup.** A confirmed, TauShell-scoped download of a pinned whisper.cpp zip and a chosen
   model, with checksum verification and re-detection before reporting success. It does not
   touch PATH and never elevates.
6. **Settings.** Enable, model size (base.en or small), language, optional CUDA build.
7. **Phone.** Later and separate; the Android companion has its own microphone handling.

## Risks

- Whisper is batch-based: text appears after you stop talking. A 10-second clip should take
  about 1-3 seconds on CPU with base or small (an estimate, not measured).
- It can invent text on silence or noise, so add a silence check and always review.
- Code identifiers and command names are often misheard; a vocabulary hint may help.
- Privacy: visible recording state, explicit enable, no audio leaves the machine by default.

## Suggested phases

1. Stage 0 check on a real window (above).
2. Spike: WebView2 microphone permission and whisper.cpp on a sample, measuring latency and
   accuracy.
3. Backend: install, transcribe and the bridge, with tests.
4. Toolbar button, hotkey, settings and composer integration.
5. Docs and a live check. Optional later: streaming and the phone.

## Out of scope

Hands-free wake words, always-on listening, spoken replies from the harness, and sending
without review.
