# Implementation Plan: Mobile Companion MVP (feature 04)

Status: **prepared for future implementation. Not started and not current scope.**
Product spec: [features/04-mobile-companion.md](../features/04-mobile-companion.md). This file turns the spec and its spikes
into ordered work. It does not change the spec; when they disagree, fix the spec first.

## 1. Definition of ready (do not start until all are true)

1. Phase 1 verification gate is closed ([02-cross-cutting.md](../02-cross-cutting.md), start gate).
2. The Phase 1 security fix for environment values on the command line is done (it shares the
   process-start code that the sidecar launch will reuse).
3. Open questions below marked **blocking** have a recorded answer in the spec.
4. Feature 02 (reliable state) is either landed, or the team accepts the MVP using the
   existing attention heuristics (`frontend/src/attention.ts`) with the documented false
   positives. Replies are always re-checked against live state on the PC either way.

### Blocking decisions

| # | Decision | Options | Needed by |
|---|---|---|---|
| D1 | TLS or plain HTTP on the LAN | Plain HTTP with documented exposure; or HTTPS with a self-signed certificate imported in the phone app (needs a spike) | WP3 |
| D2 | How the phone finds the PC after an address change | Re-pair; user-set DHCP reservation hint; mDNS name (to be measured) | WP5 |
| D3 | Verified download as a new install source | Accept; or vendor link with manual install only | WP2 |
| D4 | iOS | Confirm out of scope for the MVP | start |
| D5 | Source of "needs input / done" events | Existing heuristics now; feature 02 later | WP6 |

## 2. Principles that every work package must respect

- Off by default; nothing listens, downloads or connects until the user turns it on.
- Windows-specific code stays behind interfaces (`_windows.go` files, as elsewhere).
- Errors are visible in the UI; no silent fallbacks; no claim of success without a check.
- No prompt text, terminal output, credentials, topics or codes in logs, diagnostics or the
  audit trail.
- Harness-independent: the companion handles sessions, not any one harness.
- Follow the existing tool, config, dialog and bound-Go-opener patterns rather than new ones.

## 3. Work packages

Order matters. Each package lists what to build, where it most likely lives, and how it is
verified. Paths are the current layout and are starting points to confirm, not contracts.

### WP1: Tool entry for ntfy (detect and capability probe)
- Add an optional, non-harness tool spec for ntfy beside the existing specs
  (`internal/tools/tools.go`): executable `ntfy.exe`, version args, `serve --help`
  capability probes (listen address, auth file, default access, cache file), a tested
  range (from 2.34.0, to be widened only after testing).
- Also detect a copy in TauShell's own tools folder (`~/.tau_shell/tools/ntfy/<version>/`).
- Surface it in the existing tools UI with the same states (missing, usable, outside range).
- Tests: spec table tests like `internal/tools/tools_test.go`; fake detection.
- Done when: missing ntfy never blocks Home; outside-range shows a warning.

### WP2: Verified-download install and update source
- New install source kind next to winget and npm: download a release archive over HTTPS from
  the vendor's GitHub releases, fetch `checksums.txt`, verify SHA-256, extract to the tools
  folder, re-detect, report honestly. Refuse and show the vendor link if verification fails.
- Update check: latest release query, installed vs available, side-by-side install on
  confirmation only; keep the old version until the new one passes detection; warn that a
  running sidecar restarts. A user-managed copy is never updated.
- Reuse the confirm-dialog and install-outcome patterns in `internal/tools/install.go` and
  `cmd/taushell/dialog_windows.go`; inject the HTTP client and file system for tests.
- Tests: unit tests with a fake release server (good checksum, bad checksum, truncated
  download, missing `checksums.txt`, extraction path traversal in the archive).
- Done when: a bad or unverifiable download leaves no partial install active (spec
  acceptance), and nothing is fetched before confirmation.

### WP3: Sidecar supervisor
- New package (for example `internal/mobile`) that owns the sidecar process: writes a
  generated server config (LAN bind address only, deny-all default, cache and auth files
  under `~/.tau_shell/mobile/`), starts it as a child, waits for the health endpoint, and
  stops it.
- Child must die with TauShell: put it in a Windows job object with kill-on-close; also
  handle a stale sidecar from a previous crash (detect and stop it by the PID it recorded).
- Choose the port (default plus fallback on clash) and the bind interface (user picks; never
  all interfaces). Re-evaluate when the network changes.
- Internal admin credential: created by TauShell, random, stored only in protected local
  state (never in YAML, logs or the UI). Use Windows DPAPI or Credential Manager behind an
  interface (see the vault feature for the shared secret-store seam).
- Tests: supervisor with a fake process runner; live opt-in test with the real binary in
  the style of `TAUSHELL_LIVE=... go test ./internal/sessionhost -run TestLiveHarness`
  (for example `TAUSHELL_LIVE=ntfy`).
- Done when: closing or crashing TauShell leaves no listening sidecar; the sidecar binds
  only to the chosen interface.

### WP4: Server-side access management
- Wrap the ntfy user, access and token commands (or its admin API) behind a small
  interface: create the internal publisher, grant anonymous read-only on one topic,
  write-only per-device reply user, remove access, delete user.
- Topic and credential generation: cryptographically random, long, never reused.
- Tests: fake runner asserting exact argv; live test asserting 403/200 matrix measured in
  the spike (anonymous write to read topic, read of other topics, reply topic, revoked).
- Done when: the access matrix from the spike is reproduced by an automated live test.

### WP5: Pairing state machine and device list
- State: idle, window open (with expiry), waiting for phone confirm, waiting for PC approve,
  provisioning, paired; plus cancelled, expired, denied, failed. Single pairing window at a
  time; single-use.
- Pairing exchange: publish the pairing notification (code, Confirm/Deny), read the pairing
  reply topic, require phone confirm **and** PC approve of the same code, then provision
  per-device topics and reply credential and send the final tap-to-finish notification.
- Device registry in runtime state (separate from YAML, as with other runtime metadata):
  id, name, paired time, state, last seen, address-changed flag. No secrets in plain text.
- Revoke: delete reply credential, stop publishing to the device topic, remove access;
  restart the sidecar if a hard cut is requested (spike showed a live stream survives access
  removal).
- Detect LAN address change and mark devices "address changed - re-pair".
- Tests: state machine table tests with a fake clock and fake server (timeouts, wrong code,
  deny, double confirm, window closing mid-flow, restart during pairing).
- Done when: every acceptance criterion about pairing in the spec has a test.

### WP6: Event publishing
- Hook "needs input" and "done" into the existing attention path
  (`cmd/taushell/attention_windows.go`, `frontend/src/attention.ts`, and
  [22-local-attention-notifications.md](../../v2/22-local-attention-notifications.md)); publish only the session name and the
  state to each paired device's topic. Reuse the existing de-duplication so a phone is not
  paged twice for one event.
- Add `notifications.mobile` to `~/.tau_shell/config.yaml` alongside toast and flash,
  default off, with a per-session mute that follows the existing pattern.
- When the PC is asleep or offline nothing is sent; the phone shows the last notification
  only (state it in the UI).
- Tests: publisher with a fake server asserting the payload has no prompt text, output or
  secrets.

### WP7: Reply handling
- Subscribe to each device's reply topic (outbound from the sidecar's own API on the PC);
  accept only replies from the per-device credential; map preset words (`continue`, `yes`,
  `stop`) to an internal guarded "send to session" call.
- Always re-check live state on the PC: refuse when the session is not waiting for an
  answer; never type into a permission prompt unless that session is in a waiting state and
  the preset maps to it; never interrupt a working turn.
- Write every accepted and refused reply to the audit trail (no text beyond the preset).
- Reuse the future guarded-action layer of features [01](../features/01-control-cli-and-api.md) / [03](../features/03-prompt-dispatch.md) when
  they exist; for the MVP implement the smallest internal function and note the seam.
- Tests: reply matrix (waiting, working, ended, unknown session, revoked device, replayed
  message, unknown preset).

### WP8: UI
- Settings > Phone: enable/disable, interface picker, status of the tool and sidecar,
  Add phone, paired-device list with Revoke, last-seen, clear error states.
- Add-phone dialog: install-the-app step, pairing QR code with countdown, the code and
  Approve/Deny, success and failure screens. Generate the QR in Go or the frontend;
  the link format is fixed by the spec. ASCII-only TauShell-owned labels.
- Any external link (app store page, F-Droid) goes through the bound Go opener limited to
  known URLs; no `target="_blank"`.
- Tests: component or bridge tests in the existing frontend style; keyboard and focus
  checks; a screenshot pass per the UI design system doc.

### WP9: Security model, docs and diagnostics
- Add the mobile section to [`11-security-model.md`](../../v2/11-security-model.md): what each party sees, the
  cleartext exposure (or TLS), the verified-download source, the internal admin credential.
- Update `01-system-architecture.md`, `22-local-attention-notifications.md`, the tools docs,
  the YAML/config schema doc, the README and the contributor guide if commands change.
- Diagnostics bundle: redact topics, codes, credentials and addresses of phones.
- Threat-model checklist walk (below) recorded in the PR.

### WP10: Hardening and live verification
- Live verification pass on a real Android phone on the real LAN (see test plan).
- Fault injection: kill the sidecar mid-pairing, change the PC address, put the PC to sleep,
  phone leaves Wi-Fi, port clash, firewall blocks the port.

## 4. Dependencies between work packages

`WP1 -> WP2 -> WP3 -> WP4 -> WP5 -> (WP6, WP7) -> WP8 -> WP9 -> WP10`.
WP8 UI shells (settings, empty states) can start after WP1 using fake data. WP6 and WP7 can be
built in parallel once WP5's device registry exists.

## 5. Test plan

- **Unit:** state machines, argv, config generation, payload redaction, reply matrix, install
  verification. All with injected clock, file system, process runner and HTTP client.
- **Live, opt-in:** `TAUSHELL_LIVE=ntfy` runs the real sidecar on loopback and checks the
  access matrix and the pairing exchange with a scripted "phone". Never runs by default.
- **Manual device matrix (record results in the PR):** Android phone on the same Wi-Fi:
  scan, confirm, approve, finish; push latency; button reply; deny; timeout; revoke;
  re-pair after an address change; PC sleep and wake; phone Wi-Fi off and on.
- **Threat checks:** unauthenticated LAN client; a client that knows only the pairing topic;
  replayed or forged reply; a revoked device replying; tampered download; archive with
  `..` paths; a second pairing window; sidecar left running after a crash.

## 6. Rollout

- Ship behind an off-by-default setting and label it experimental in the first release.
- Android only. Document that iOS, away-from-home, TLS (if deferred) and live terminal are
  not part of this release.
- Keep the old attention paths (toast, flash) unchanged; mobile is additive.

## 7. Risks to watch during implementation

- Cleartext LAN exposure if D1 ends as plain HTTP.
- The ntfy CLI/admin surface changing between versions: capability probes and the tested
  range exist for this reason; failures must be visible.
- Windows firewall behaviour on the user's network profile differs by machine; the test here
  did not need a rule, others may.
- Phone battery optimization causing late notifications; the setup guide must say so.
- Scope creep into live terminal view or remote access; reject in review.

## 8. Definition of done

- All spec acceptance criteria pass, each mapped to a test or a recorded manual check.
- Security model updated; threat checklist recorded; no secret appears in logs or
  diagnostics (verified by an automated redaction test).
- Tool install and update verified with a bad-checksum case.
- A recorded real-device run on the LAN with the results from section 5.
- Docs listed in WP9 updated; spec "spikes" and "open questions" updated to final answers.
