# 04 - Mobile Companion (MVP)

Status: **future planning**. Rank 4. Sources: V3 ideas #7; Orca mobile companion. Out of Phase 1 scope.

## Problem

Agents run for a long time. Leaving the desk means missing the moment an agent needs an
answer or finishes. Windows toasts only help in front of the PC.

## Scope decision

The minimum useful version is **push notifications plus quick reply and action buttons**,
over the **local Wi-Fi/LAN only** (decision, 2026-10-29: for security and simplicity). The
transport is **a TauShell-managed ntfy sidecar process** bound to the LAN; no internet
relay, no cloud, no remote access. A live terminal view, restore from the phone, and
away-from-home delivery are later steps.

## User stories

- As a user away from my PC, I want a notification when a session needs me or finishes, so
  that I can return or answer.
- As a user, I want to reply from the phone with a short answer or a preset action
  (for example "continue", "yes", "stop"), so that a stuck agent moves on.
- As a user, I want to pair and revoke devices, so that only my phone can reach TauShell.

## In scope

- Pairing a phone with the desktop app (QR code with the LAN address and credentials);
  listing and revoking paired devices.
- A managed ntfy sidecar, tracked as one of TauShell's **tools** next to PowerShell and the
  harnesses (see "ntfy as a tracked tool" below): detected, versioned, installable and
  updatable with user confirmation, run as a child process that TauShell supervises and
  stops with the app.
- The sidecar binds only to the one LAN interface the user picks, never all interfaces, and
  only while the feature is on.
- Push of "needs input" and "done" events with the session name only.
- Quick reply and action buttons that map to the same guarded actions as
  [features 01 and 03](01-control-cli-and-api.md).
- A session overview with state.
- Clear off switch; off by default.

## Out of scope for the MVP

- Live terminal view, restore and start from the phone.
- Anything that works away from the local network (public relay, internet, mesh VPN).
- iOS (a self-hosted ntfy cannot give instant iOS push without an internet relay).
- Cloud accounts, sync or hosted dashboards.
- Sending terminal content or prompts to third-party services.
- Embedding ntfy as a library inside the TauShell process, or shipping the ntfy binary with
  TauShell by default.

## Acceptance criteria

- Nothing listens or connects out until the user turns the feature on.
- The sidecar listens only on the chosen LAN interface and requires authentication
  (deny-all by default); an unauthenticated request from the LAN is refused.
- A device that was not paired cannot read state or send input; a revoked device loses
  access immediately.
- The reply credential held by the phone can only write to the reply topic.
- Notifications contain no prompt text, terminal output or credentials.
- A quick reply is refused when the session is not waiting for an answer, and every reply is
  in the audit trail.
- Closing TauShell, or crashing, leaves no orphaned sidecar listening.
- Transport and data flow are documented in the security model, with what each party sees.
- If the desktop is asleep, off, or the phone is off the LAN, the phone shows the last known
  state where possible and the UI says delivery is unavailable.

## Dependencies

- [01](01-control-cli-and-api.md) (control channel), [02](02-reliable-state-and-inbox.md) (state and inbox).
- [Findings 2.2 and 2.4](../02-cross-cutting.md#24-first-network-surface). Transport spike.

## Transport decision

**Chosen: LAN only, ntfy sidecar.** Options not chosen are kept for later phases.

| Option | Status |
|---|---|
| LAN only with a managed ntfy sidecar | **Chosen for the MVP** |
| ntfy library embedded in TauShell | Rejected: shares the process that holds every session, forces TauShell releases for each ntfy fix, adds dependencies (SQLite/cgo to be verified) and license review |
| Public ntfy.sh or other internet relay | Later; a third party sees event metadata |
| Private mesh VPN (for example Tailscale) | Later; would give away-from-home use and encryption |
| Native or PWA app through a cloud relay | Later or never; largest trust cost; PWA push also needs HTTPS |

## One-touch pairing (Bluetooth-style, no typed credentials)

Decision (2026-10-29): the user never types or sees a username or password. Pairing works
like Bluetooth: the user starts it on the PC, acts on the phone, confirms a short code on
both sides, and TauShell provisions everything else itself. Secrets are random values that
move only inside the pairing exchange.

**User flow**
1. PC: *Settings > Phone > Add phone*. TauShell checks the sidecar is installed and running
   on the chosen LAN interface (offers the confirmed install otherwise) and opens a pairing
   window. The dialog shows step 1 "Install the ntfy app" (a QR link to the store/F-Droid
   page, skipped when already installed) and step 2 the **pairing QR code**, with a
   countdown (for example 3 minutes).
2. Phone: scan the QR code with the camera. It is an `ntfy://<pc-lan-address>:<port>/<random
   pairing topic>?secure=false&display=TauShell` link, so the ntfy app opens and subscribes
   with no login screen (verified on a real phone: see Spikes).
3. Phone and PC: TauShell publishes a "Pair with <PC name>?" notification with a 4-digit
   code and **Confirm** / **Deny** buttons; the PC dialog shows the same code and an
   **Approve** button. Pairing completes only when the phone taps Confirm **and** the user
   approves the same code on the PC (this is the numeric-comparison step that stops someone
   else on the LAN from joining while the window is open).
4. TauShell provisions the device: a random per-device notification topic and a random
   per-device reply topic, and a write-only reply credential for that reply topic only.
   The phone is moved onto them by a final notification whose button opens an `ntfy://`
   link to the new topic, and the pairing topics are deleted. The device then appears in
   the paired-device list with a name, the pairing time and a Revoke button.

**Rules**
- The pairing window is time-limited and single-use: it closes after one successful pairing,
  on timeout, on Cancel, or when TauShell closes. Pairing topics have no access afterwards.
- No account, password or token is ever typed, displayed or logged. The reply credential is
  generated by TauShell and delivered only inside the notification action, never in the QR.
- A scanned QR on its own grants nothing beyond reading the pairing topic; it cannot send
  input or read session state before the code is approved on the PC.
- Revoke removes the device's topics and credential on the server; the phone stops
  receiving, and a reply with the old credential is refused immediately. (Measured: removing
  topic access alone does not close a live stream, so revoke must also stop publishing to
  the device topic and delete the reply credential.)
- Every pairing, approval, denial, timeout and revocation is written to the audit trail
  without codes, topics or credentials.
- Re-pair is the same flow; if the PC's LAN address changes the device is shown as
  "address changed - re-pair" rather than failing silently.

**What the user does:** scan, tap Confirm on the phone, click Approve on the PC (one
confirm each side), then tap once on the final notification to finish. This is "few-touch",
not literally one touch, because the ntfy app cannot be driven further than a deep link.
A custom companion app could make it one touch later; it is out of scope for the MVP.

## ntfy as a tracked tool

Decision (2026-10-29): ntfy joins the tool list that `internal/tools` already detects
(PowerShell 7 and the harnesses) and follows the same rules from `AGENTS.md`: detect, show
the installed version, warn outside the tested range, offer a confirmed install, offer a
confirmed update, never install, elevate, pin, downgrade or auto-update silently, and
missing ntfy never blocks Home or any other feature.

- **Role:** an optional tool, not a harness and not required. Only the mobile companion
  needs it. It appears in the same tools UI with state (missing, usable, outside the tested
  range, update available) and "Install" / "Update" actions.
- **Detection:** look for `ntfy.exe` on PATH and in TauShell's own tools folder; run its
  version command; probe `ntfy serve --help` for the options TauShell relies on (listen
  address, auth file, default access, cache file) as capability probes. A missing required
  option makes the tool unusable for this feature and says which one.
- **Existing mechanism and the gap:** today a tool declares a winget package or an npm
  package, plus a vendor URL as fallback. There is **no maintained winget package for ntfy**
  (`winget search ntfy` found none on the test machine), and the vendor ships a Windows
  `.zip` on GitHub releases. So this feature needs a third install source: a **verified
  release download**.
- **Not shipped with TauShell** (decision, 2026-10-29): ntfy is never bundled in the
  installer or the app. Installing it is an opt-in action available later, like any other
  tool, only when the user turns the mobile feature on or installs it from the tools list.
- **Assisted install:** after the user confirms, TauShell downloads the official release
  archive from the vendor's GitHub releases over HTTPS, verifies it against the published
  checksum when the release provides one (to verify; refuse and show the vendor link if it
  cannot be verified), extracts it into a TauShell-owned folder under `~/.tau_shell/tools/`
  (no elevation, no PATH change), re-runs detection, and reports success or failure
  honestly. The vendor page stays the fallback.
- **Assisted update:** an update check queries the vendor's latest release (no visible
  window, rate-limit tolerant, off the network when the feature is off), shows installed
  versus available, and installs the new version side by side only on confirmation; the
  previous version stays until the new one passes detection. If the sidecar is running the
  user is told it will be restarted.
- **A user-managed ntfy** (own install, other path) is detected and used but never updated
  by TauShell.
- **Tested range:** declared like harnesses (from inclusive, before exclusive); the measured
  spike used 2.34.0.
- **Acceptance:** nothing is downloaded until the user confirms; a failed or unverifiable
  download leaves no partial install active; the installed version and its source are
  visible; update never runs automatically; removing the feature leaves the tool folder
  removable from the UI.

## Risks

- First network surface (an inbound LAN listener); the largest security risk in V3.
- Plain HTTP on the LAN exposes the reply token; TLS with a certificate the phone trusts is
  unresolved (see open questions).
- The phone must find the PC: IP changes and DHCP leases break paired addresses.
- Windows firewall prompt and per-network profile rules (public networks must not allow it).
- Phone OS background restrictions and OEM battery killers.
- Sidecar supervision: orphaned process after a crash, port clashes, version drift.
- Downloading and running a third-party executable: supply-chain trust, so verified
  checksums, an official source only, and a visible version/source.

## Spikes

- **Done (desk research only, nothing measured on a phone): transport shortlist.**
  No phone or relay was tested; no data left this machine. Sources: ntfy docs
  (<https://docs.ntfy.sh/publish/>, <https://docs.ntfy.sh/config/>).
  - **ntfy (self-hostable push relay)** fits the MVP shape best: a plain HTTP POST from the
    desktop publishes a notification with a title, priority and **action buttons** (HTTP
    actions), so "push + quick reply buttons" needs no custom phone app. Findings to respect:
    on the public server the topic name is effectively the password (no sign-up), so it must
    be long and random, or use a self-hosted server with auth (`auth-default-access: deny-all`);
    the relay sees title and body, so only the session name and state may be sent
    (acceptance criterion: no prompt text or output); a button's HTTP action calls a URL that
    must be reachable from the phone, so replies still need a LAN/VPN path back to TauShell
    or a second ntfy topic the desktop subscribes to.
  - **Superseded recommendation (2026-10-29):** the desk-research direction below (outbound
    only, public ntfy.sh default, second topic subscribed by the desktop) assumed an internet
    relay. The LAN-only decision replaces it: TauShell hosts ntfy, so there is an inbound LAN
    listener; the public-relay and second-topic parts are deferred.
  - **Recommended direction for the MVP (to confirm with a real phone):** ntfy for outbound
    events and for replies over a second authenticated topic the desktop subscribes to, so
    TauShell opens **no inbound port at all**. This keeps feature 04 free of a listening
    network surface; only an outbound connection exists, and only when the user turns it on.
    Pairing = a QR code carrying server URL, random topic names and a reply token; revoke =
    rotate topics and token.
  - **Self-hosted vs public relay:** offer public ntfy.sh as the zero-setup default with a
    clear disclosure of what the relay sees, and a self-hosted URL field for users who want
    none. Replies must be signed with the pairing token, because anyone who learns a topic
    can publish to it.
  - **Rejected for the MVP:** a PWA/native app through our own cloud relay (we would operate
    a service; largest trust cost) and exposing a TauShell port on the LAN (inbound surface,
    Windows firewall prompt, works only at home). A mesh VPN stays an optional add-on for the
    later live-terminal step.
  - **Done (measured on a real Android phone): ntfy push + action buttons.**
    Samsung Galaxy A52, Android 14; self-hosted ntfy 2.34.0 on the PC (deny-all, one user);
    ntfy Android app 1.25.2 (F-Droid build, no Google push), reached over USB (`adb reverse`).
    Lab conditions: same machine, no real network path, so latency is a lower bound.

    | Check | Result |
    |---|---|
    | Anonymous access to a deny-all server | Refused (403/401); the phone had to log in. |
    | Notification with title, high priority and two HTTP action buttons | Appeared on the phone in under 1 s (poll-bound measurement) with both buttons. |
    | Tapping "Continue" | The phone POSTed `continue` to a separate reply topic with the supplied auth header; the desktop side read it back. The quick-reply loop works with no custom app. |
    | Deep doze forced (`deviceidle force-idle`, screen off, unplugged state simulated) | Notification still delivered in under 1 s. Not proven for hours of real idle or OEM battery killers. |

    Findings for the spec:
    1. The reply button's `Authorization` header is stored in the notification payload, so the
       relay and anyone who can read the topic can see it. The reply credential must be a
       per-device, write-only token for the reply topic, never a general account password,
       and revocable by rotating it.
    2. The reply carries only a preset word (`continue`, `stop`); TauShell must still check
       that the session is waiting and log it, since the phone cannot be trusted to know.
    3. The app showed a battery-optimization warning; real delivery on Samsung/other OEMs needs
       the user to exempt it. Document this in setup.
    4. The user's lock-screen shows notification text; keep the content to session name and
       state only.
    5. Over the internet a reply URL must be reachable from the phone (the second-topic design
       in the recommendation avoids opening a port on the PC).
    Remaining unmeasured: iOS, real-network latency, multi-hour idle reliability, Google-push
    (Play) build, and the public ntfy.sh behaviour. The second topic subscribed by the desktop
    (instead of an HTTP action to the PC) was designed but not built.
- **Done (measured on a real phone over the real Wi-Fi): pairing mechanics.**
  Same Samsung A52; ntfy 2.34.0 on the PC bound to its LAN address only
  (`192.168.2.151`, network profile *Public*), phone on the same /24 (`192.168.2.103`),
  plain HTTP, app 1.25.2. QR scanning itself was not automated; the same `ntfy://` link a
  QR would carry was opened on the phone with an Android VIEW intent.

  | Check | Result |
  |---|---|
  | Phone reaches the sidecar over Wi-Fi on a *Public* profile, unelevated, specific-IP bind | Yes (HTTP 200 on the health endpoint). No firewall change was needed on this machine; a prompt or rule may still appear elsewhere. |
  | `ntfy://<ip>:<port>/<topic>?secure=false` deep link | The app opened and subscribed with **no login screen**. |
  | Anonymous read-only on the one random topic, deny-all elsewhere | Anonymous write to that topic, read of the reply topic and read of any other topic all returned 403. |
  | Pairing notification with Confirm / Deny buttons, app in background | Visible in about 0.8 s. Tapping Confirm posted `confirm:<code>` to a write-only pairing reply topic, which the PC read back. |
  | Final step: tapping a notification whose click target is an `ntfy://` link to a new topic | The app subscribed to the new per-device topic. This is the "tap once to finish". |
  | Topic access removed (`--reset`) while the phone was subscribed | New requests got 403, **but the already-open phone stream kept receiving**. |
  | Notification shown while that topic is open in the app | Suppressed; the message appears in the app list only. |

  Findings for the spec:
  1. The PC must publish with its own internal credential (an admin user created by
     TauShell and kept out of the UI and logs); anonymous read-only topics cannot be
     published to. This is not a user credential.
  2. Removing access does not cut a live stream. Revoke must therefore also stop publishing
     to that device's topic and delete the device's reply credential (a per-request check,
     so the reply path closes immediately), and restart the sidecar if a hard cut is needed.
  3. The ntfy release publishes `checksums.txt` (no signature). The Windows archive matched
     it. That proves integrity of the download, not authenticity beyond GitHub itself; say
     so in the security model.
  4. HTTP on the LAN means the pairing topic, per-device topic and reply credential cross the
     network in cleartext; the exposure window is short for pairing but permanent for the
     device topic. Decide on TLS (open question) before shipping.
  5. Not measured: iOS, camera QR scan end to end, sidecar behaviour across PC sleep, Wi-Fi
     roaming, and an address change.

- **New spikes for the LAN design:** (a) sidecar on a real Wi-Fi path between the PC and the
  phone: latency, reconnect after Wi-Fi sleep, PC sleep/resume, DHCP address change;
  (b) TLS the Android app will accept (self-signed with a pinned/imported certificate versus
  plain HTTP with a short-lived token) and what the QR pairing must carry; (c) Windows
  firewall behaviour: a rule scoped to the chosen interface and the Private profile only,
  without elevation; (d) sidecar lifecycle: kill-on-exit via a Windows job object, port
  selection, version/capability probe.
- **Tool install spike:** does each ntfy release publish checksums, and can the archive be
  verified without extra tools? Where is the extracted tool stored and how are old versions
  removed? Which ntfy license applies, and what is the binary size?
- Pairing flow (QR code) and key storage.

## Open questions

- Plain HTTP on a trusted home LAN or mandatory TLS? Both need an Android-side check.
- How is the PC found when its LAN address changes (re-pair, mDNS, user-set name)?
- Is a verified GitHub-release download acceptable as a new install source, given that
  ntfy has no winget package? (Alternative: vendor link only, manual install.) Bundling is
  already ruled out.
- Android only for the MVP: confirm iOS is acceptable to leave out.
- Should quick replies be limited to a preset list?

## Docs to update when implemented

Implementation plan (ready/done criteria, work packages, test plan): [../implementation/04-mobile-companion-plan.md](../implementation/04-mobile-companion-plan.md).

`11-security-model.md` (new section; verified-download install source), `01-system-architecture.md`, `22-local-attention-notifications.md`, `23-adding-a-harness.md` or the tools docs (non-harness optional tool), README.
