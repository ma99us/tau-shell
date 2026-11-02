# Implementation Plan: Mobile Companion MVP (feature 04)

Status: **decisions recorded; transport spike done; implementation not started. Not Phase 1 scope.**
Product spec: [features/04-mobile-companion.md](../features/04-mobile-companion.md). The spec was written around an ntfy
sidecar and is superseded in part by this plan (see its banner). The earlier ntfy-based work packages were removed
because decisions D1 and D3 replace them with a custom Android app and a custom pinned-TLS endpoint.

## 1. Decisions (recorded 2026-11-02, user)

| # | Decision | Answer | Consequence |
|---|---|---|---|
| D1 | Transport security | A per-pairing certificate that the app pins (no system trust store, no user cert import). Confirmed by the user as a per-pairing interpretation. | A shared key in the APK would let anyone who unpacks it impersonate the PC, so each pairing generates its own key and token. Delivered by QR; revoking a device discards its credentials. |
| D2 | Reaching the PC | Phase 1: local LAN only, no mDNS (address comes from the QR). Later phase: a small serverless relay (Cloudflare Developer Platform or AWS Lambda) where TauShell registers itself and the phone talks to the same relay. | The relay is a third-party surface. It must carry only ciphertext (end-to-end encrypted with keys from pairing), and be enabled only after an explicit consent screen. Needs its own security spike. |
| D3 | Android install | Sideload an APK for now. | Publish a SHA-256 of the APK and a sideload warning. No store. |
| D4 | iOS | Out of scope. | None. |
| D5 | Event source | `internal/agentstate` (confirmed state; only Claude Code and Copilot CLI have cues). | Sessions without a cue show "unknown" on the phone and cannot be driven. |

## 2. What already exists

- `internal/agentstate`: confirmed working, input, idle, unknown, stopped, with source and staleness rules.
- `internal/control`: guarded `list`, `wait`, `send` (submit or insert-only, 16 KiB cap, refusal rules) and `start`, with an audit trail that holds session ids and outcomes only. Verified live against Copilot CLI.
- `internal/pairing` (spike, not wired in): per-pairing ECDSA P-256 certificate, 256-bit bearer token, TLS 1.3 only, client-side SHA-256 pin. Tests cover wrong token, wrong pin, unpinned client, expired certificate and TLS 1.2.

The phone server is a **second front end for the same `control.Sessions` logic**, not a new path to the terminal. Every action goes through the same refusal rules and the same window re-check.

## 3. Principles for every work package

- Off by default; nothing listens until the user turns it on and pairs a phone.
- Windows-specific code stays behind interfaces (`_windows.go` files).
- Errors are visible in the UI; no silent fallbacks; no claim of success without a check.
- No prompt text, terminal output, tokens, fingerprints, pairing codes or phone addresses in logs, diagnostics or the audit trail. Audit records the device id, session id, action and outcome only.
- The phone never gets terminal content in this MVP: session names, states and the actions below only.
- Harness-independent: the server handles sessions, not any one harness.
- Reuse the existing config, dialog and bound-Go-opener patterns.

## 4. Work packages

Order matters. Paths are starting points to confirm, not contracts.

### WP1: Pairing credentials and device registry
- Grow `internal/pairing` from the spike: persist each device's certificate fingerprint hash, token hash (store only a hash of the token), name, paired time, last seen and state in runtime state (not YAML). The private key and token are shown to the phone once.
- Certificate lifetime and rotation: choose a lifetime (for example one year), show expiry in the UI, and re-pair to renew. Revoke = delete the device record and key.
- Protect the private key on disk with DPAPI (or Credential Manager) behind an interface.
- Tests: registry round trip, revoke, expired device, token stored only as hash.
- Done when: a revoked or expired device cannot authenticate and no secret appears in files in plain text.
- Status: built in `internal/pairing/registry.go` (JSON file written with `fsutil.ReplaceFile`; token stored only as SHA-256; private key sealed through a `Sealer` interface; a corrupt file is an error, not an empty registry). **Not done:** the Windows DPAPI `Sealer` and the file location in `~/.tau_shell`; both belong to the wiring step.

### WP2: Pairing flow
- Pairing window: single, short-lived, single-use, started by the user in the UI. The QR holds the LAN address and port, the certificate fingerprint and a one-time pairing code.
- Exchange: the phone connects with the pin from the QR and presents the pairing code; the PC shows the device name and asks the user to Approve in the UI; on approval the phone receives its bearer token. Wrong code, a second attempt, a closed window and a denied request all fail visibly.
- Tests: state machine table tests with a fake clock (timeout, wrong code, replay, double confirm, window closes mid-flow, restart during pairing).
- Done when: every pairing failure mode has a test and a PC-side message.
- Status: state machine built in `internal/pairing/window.go` with fake-clock tests (deny, wrong code, five wrong codes close the window, expiry mid-flow, single use, a second phone refused while one waits, reopening cancels a waiting request). The flow runs end to end in `internal/mobile/server_test.go`. **Not done:** the QR payload format and the PC-side Approve dialog (WP6).
- Design choice: the pairing channel and each paired device use different certificates, selected by the TLS server name: `pair` for the one-time pairing certificate (created when the window opens), then `d-<device id>` for that device's own certificate. The phone pins the pairing certificate from the QR, receives its own fingerprint and token over that pinned channel after approval, and reconnects with its own name. This keeps one key per pairing (D1). The Android client must set the TLS server name explicitly.

### WP3: Phone API server
- New package `internal/mobile`. HTTPS server (TLS 1.3, pinned cert, bearer token required on every route) bound to the interface the user picks, never all interfaces. Port default with fallback on clash. Re-evaluate on network change; mark devices "address changed" so the user can re-pair.
- Routes (v1, JSON):
  - `GET /v1/sessions`: names, id, running, `agentstate.View`.
  - `GET /v1/events`: long poll or server-sent events with state changes and "needs input" or "done" events (session name and state only). Resumable with a last-event id, with bounded buffering.
  - `POST /v1/sessions/{id}/send`: `{text, insertOnly}` through `control.Sessions.HandleSend`. Typed prompts and presets use this same guarded path.
  - `POST /v1/sessions/{id}/start`: through `HandleStart`.
- Rate limit per device and cap request size; constant-time token compare; a failed-auth backoff.
- Server stops when the setting is off, and with the app (no sidecar process; it lives in the TauShell process, so nothing is left running after a crash).
- Tests: handler tests with the fake `Sessions` backend (auth matrix, refusal passthrough, oversize, bad UTF-8, rate limit); an end-to-end test with the pinned test client from the spike.
- Done when: a scripted client reproduces list, wait-for-event and a guarded send with correct refusals.
- Status: built in `internal/mobile/server.go` with end-to-end tests over a real pinned TLS listener (`server_test.go`): pairing, list, guarded send (409 while working, insert-only allowed, 404 unknown session, no text in the audit), revoke, token and certificate mix-ups, TLS 1.2 refused, failure back-off (10 per minute per address, then 429). Routes built: `POST /v1/pair`, `GET /v1/sessions`, `GET /v1/sessions/{id}/wait`, `POST /v1/sessions/{id}/send`, `POST /v1/sessions/{id}/start`. **Not done:** the global `GET /v1/events` stream (the per-session `wait` long poll covers it for now), a per-device request rate limit, interface binding and port selection, and wiring it to the app and to `control.Sessions` (WP6).
- Found while testing: the back-off tracker wrote to a nil map and panicked on the first request. Fixed, covered by the tests.

### WP4: Preset and typed replies
- Presets are configured on the PC (YAML or UI), for example "continue", "go ahead", "summarize"; they are sent through the guarded `send`, so they only work on a confirmed-idle session with no draft.
- Typed prompts use the same endpoint with the same checks. The text is never logged or audited.
- **Answering a permission prompt is a separate operation** and is not covered by `send`: `send` refuses a confirmed `input` state. Add an `answer` op that maps a fixed preset (`yes`, `no`, `stop`) to fixed keys, only for a confirmed `input` state, per-harness (adapter-owned) and verified live for each harness first. A typed answer (a choice or a reason) needs per-harness rules and is a user decision; leave it out of the MVP until decided.
- Tests: reply matrix (idle, working, input, unknown, stopped, draft present, revoked device, replayed request).
- Done when: the matrix passes, and a live run against Copilot CLI answers a real permission prompt only when confirmed.

### WP5: Android app (sideload APK)
- Native Kotlin app, minimum scope: scan the QR, store the pin and token in the Android Keystore, session list, event stream, preset buttons and a text box, clear error states.
- TLS: a custom `X509TrustManager` (or OkHttp pinning with a custom trust manager, since the certificate is self-signed) that accepts only the pinned SHA-256 and a TLS 1.3 minimum. Do not fall back to the system store.
- Notifications on a LAN-only design have no push service (no FCM, to stay local): use a foreground service that holds the event stream, and document battery optimization. Fall back to a poll on app open.
- Build and sign locally; publish the APK SHA-256 and a sideload guide. No store listing.
- Tests: unit tests for the pin verifier and the request signing; an instrumented run on a real phone on the LAN.
- Done when: the manual device matrix in section 6 passes.

### WP6: Desktop UI and config
- Settings > Phone: enable switch (off by default, persisted, like the control channel switch), interface picker, status, Add phone, device list with last seen and Revoke.
- Add-phone dialog: QR with countdown, device name, Approve and Deny, success and failure screens. ASCII-only TauShell-owned labels. Any link goes through the bound Go opener.
- Add `notifications.mobile` to `~/.tau_shell/config.yaml` next to toast and flash, default off, with a per-session mute that follows the existing pattern. Reuse the existing de-duplication so the phone is not paged twice for one event.
- Tests: component and bridge tests in the existing frontend style.
- Status: wired into the app. `cmd/taushell/mobile_windows.go` starts the server only when `mobile.enabled` is true and `mobile.interface` is a local address (never all interfaces); `StartPairing`, `DecidePairing`, `CancelPairing`, `RevokePhone`, `GetMobileStatus` and `SetMobileEnabled` are the bound methods. The pairing QR is drawn in Go (`github.com/skip2/go-qrcode`) and encodes `taushell://pair?host=&port=&fp=&code=`. The Home Phone pane (`frontend/src/phonePane.tsx`) has the switch, the device list with Revoke, Add phone with the QR, the Approve and Deny prompt and a short audit list. The DPAPI sealer is `internal/pairing/dpapi_windows.go`; devices live in `~/.tau_shell/mobile/devices.json`. Tests: `cmd/taushell/mobile_windows_test.go` (explicit local interface, pair, revoke, stop closes the port, switch persisted), `internal/appconfig/mobile_test.go`. **Not done:** an interface picker (the address is set by hand in config.yaml), `notifications.mobile` with a per-session mute, a live GUI check, a component test for the pane, and the Android app.

### WP7: Security model, docs and diagnostics
- Add the phone section to [`11-security-model.md`](../../v2/11-security-model.md): what each party sees, LAN exposure, the pin, token hashing, revocation and rotation.
- Update the architecture doc, `22-local-attention-notifications.md`, the config schema doc and the README.
- Diagnostics bundle redacts tokens, fingerprints, pairing codes and phone addresses (automated redaction test).
- Record a threat-model walk in the PR: unauthenticated LAN client, a client that knows only the fingerprint, forged or replayed request, revoked device, expired certificate, a second pairing window, port scan, oversize and malformed input.

### WP8: Hardening and live verification
- Real Android phone on a real LAN. Fault injection: change the PC address, PC sleep and wake, phone leaves Wi-Fi, port clash, Windows firewall blocks the port, app closed mid-send.

### WP9 (later): Relay (D2 phase 2)
Not part of the MVP. Preconditions: a security spike and a consent screen.
- The relay (Cloudflare Worker with Durable Objects, or AWS Lambda behind an API) only forwards opaque messages between a PC and its paired phones, keyed by a per-pairing id.
- End-to-end encryption with keys derived at pairing; the relay holds no plaintext and no keys. Sessions, states and replies are all inside the ciphertext.
- Replay protection, message expiry, rate limits and abuse controls; no persistent storage of message bodies.
- TauShell registers outbound (no inbound port), only when the user enables it after an explicit consent screen that says what leaves the PC.
- The LAN server and the relay share the same message schema and the same `control.Sessions` path.

## 5. Dependencies

`WP1 -> WP2 -> WP3 -> (WP4, WP5) -> WP6 -> WP7 -> WP8`. WP6 shells with fake data and WP5 against a test server can start after WP3 has stable routes. WP9 follows a security spike after the MVP.

## 6. Test plan

- **Unit:** pairing state machine, registry, handlers, refusal matrix, payload redaction, rate limit, all with an injected clock.
- **End to end (Go):** pinned client against the real server and a fake `Sessions` backend; extend the spike tests.
- **Live, opt-in:** run the real server on LAN with the Android app against a disposable Copilot CLI session (never the user's own sessions).
- **Manual device matrix (record in the PR):** scan, approve, list, event latency, preset reply, typed reply, refusal while working, revoke, re-pair after an address change, PC sleep and wake, phone Wi-Fi off and on, certificate expiry.

## 7. Rollout

- Off by default and labelled experimental. Android sideload only. iOS, away-from-home use (relay) and live terminal view are not part of this release.
- The existing toast and flash paths stay unchanged; mobile is additive.

## 8. Risks

- Android battery optimization delays the foreground service; the guide must say so.
- Only Claude Code and Copilot CLI have confirmed state; the other harnesses show "unknown" and cannot be driven from the phone.
- Copilot's idle footer disappears while a draft is typed, so a drafted session is not drivable.
- A phone on the LAN can still be lost or stolen: revoke must work from the PC, and the token must be stored in the Android Keystore.
- Windows firewall rules differ per network profile.
- Scope creep into terminal view or remote access; reject in review.

## 9. Definition of done (MVP)

- Every pairing and refusal criterion maps to a test or a recorded manual check.
- Security model and docs updated; threat checklist recorded; redaction test passes.
- A recorded real-device run on the LAN.
- The spec's "spikes" and "open questions" are updated to these answers.
