# Validation and decision gates

Status: proposed backlog for an unimplemented study. No gateway compatibility
test or live provider experiment below has been run. Documentation approval
authorizes none of the installations, logins, paid usage or product changes.

## Gate 1: policy, access and billing

Before an experiment, identify:

- Exact corporate provider, account, payer, permitted models and data boundary.
- Whether corporate Copilot allows the proposed third-party client/auth path,
  not merely whether a token can authenticate.
- Whether a GitHub App route is separately organization-billed rather than
  consuming an existing user allowance.
- Whether OpenAI access is an API account, a subscription, or both.
- Managed harness policies that prohibit changing endpoints or providers.

Pass: the owner approves a named route and synthetic test traffic, including any
cost. Otherwise leave it blocked, not automatically replaced by a personal
account or different provider. This gate depends on the
[feasibility findings](01-feasibility.md).

## Gate 2: smallest reproducible local configuration

Depends on Gate 1 for live access; offline packaging checks can run separately
after approval.

Record exact harness/gateway versions, sources, licenses, supported Windows
runtime and required optional dependencies. Establish whether the selected auth
path needs database/dashboard features; do not assume all LiteLLM setups do.
Verify download integrity, executable detection, version reporting, readiness,
port-conflict handling and owned-process cleanup.

Pass: installation is confirmed, reproducible and removable without elevation,
global PATH edits or native harness config changes. No claims about other
versions or provider combinations follow from one successful setup.

## Gate 3: non-invasive launch and credential precedence

Depends on a chosen harness/version and proposed configuration mechanism.
Use disposable user profiles with synthetic credentials and fixtures, not a
user's real credential store or conversation history.

| Case | Required result |
| --- | --- |
| Ordinary launch, stop, startup failure and crash | Global/project provider config, stored auth defaults, shell profiles and registry environment remain unchanged |
| Native launch before/during/after a TauShell launch | Native route/account/settings remain unchanged, including when the proxy is unavailable |
| Two simultaneous TauShell profiles | Each reaches only its pinned upstream/account; no host-wide env mutation |
| Global/project/local settings conflict | Effective precedence is demonstrated; no accidental direct route or saved-login fallback |
| Mandatory managed-policy conflict | Launch is blocked visibly; managed restrictions are not bypassed |
| Existing skills/hooks/plugins/MCP/permissions | Existing functionality remains present, subject to unchanged policy |
| Secret injection | No secret in argv, overlay files, logs, YAML, URLs, UI or state DB |
| Claude background/supervisor mode | It uses only the approved route with secure credentials, or the mode is explicitly unsupported |
| Resume and auxiliary readers | Same native conversation context and correct discovery/model/title/activity/usage/config lookup |
| Missing secure override support | Only the opted-in launch fails; defaults are untouched |

In fixture profiles, compare default config/auth bytes and relevant environment
before and after each case. Distinguish expected conversation/log changes from
forbidden default-configuration changes. Never use backup/edit/restore as the
test implementation.

Pass: exact effective endpoint, account and credential source are demonstrated
without exposing secret values. A successful child environment merge alone is
not proof that the harness honors it.

## Gate 4: protocol, identity and quota fixtures

Depends on the selected route and Gate 3's secure configuration. Use local fake
upstreams before spending provider quota.

| Area | Acceptance criteria |
| --- | --- |
| Anthropic streaming | Correct SSE ordering/end markers, incremental delivery, keepalives, cancellation and disconnect behavior |
| Tools | Tool calls and results round-trip with correct IDs, parallel calls and error handling; no unintended replay |
| Capabilities | Thinking, structured output, context management and required beta fields work or produce an explicit unsupported-capability block |
| Context and auxiliary requests | Actual context limit, token counting, main/small/background model aliases and compaction are verified |
| Headers and usage | Required version/beta/cache headers and actual usage survive; no fictitious zero costs |
| Auth failures | Missing/expired named credentials never use shared login, stored subscription auth or another account |
| Billing isolation | Subscription failure never triggers paid API or personal-account fallback |
| Quota evidence | Unknown/stale/exhausted/unlimited states remain distinct; account, unit, scope and timestamp match the route |
| Provider errors | Auth/policy/budget/429/network errors stay distinguishable and visible |
| Concurrency | Simultaneous fixed routes do not leak credentials or conversation contents across profiles |

Pass: fixtures assert upstream destinations and zero unauthorized fallback
attempts, not just final answer text. A reported allowance must not be treated as
a guarantee that the next request succeeds.

## Gate 5: local service and data boundaries

Depends on Gates 2-4 before enabling a managed gateway:

- Prove authenticated loopback-only binding and rejection of unauthenticated
  clients, arbitrary host/origin access, unknown profiles and arbitrary upstreams.
- Verify upstream TLS/corporate CA behavior, redirect refusal, size/concurrency
  bounds and cancellation.
- Inspect synthetic failure diagnostics for secret/prompt/tool-content leakage.
- Test port conflicts, sidecar crashes, app shutdown with running sessions,
  restore after restart and cleanup of owned artifacts.
- Stop only the owned sidecar after dependent sessions stop; never disrupt an
  externally managed service or native harness.

Pass: failures remain local to opted-in sessions and do not alter global
configuration. Same-user credential access remains a documented limitation,
not a claimed sandbox guarantee.

## Gate 6: separately approved live end-to-end verification

Depends on Gates 1-5. Use disposable working directories, approved accounts,
synthetic prompts, bounded spending and per-route consent. Do not modify real
projects or print credentials.

For each exact harness/provider/model/version combination:

1. Launch through TauShell/ConPTY and verify the actual provider/account/model.
2. Run a small tool-assisted task, including a permission prompt and cancellation.
3. Check small/background requests and compaction where supported.
4. Stop gracefully, restore the same conversation and demonstrate context recall.
5. Compare available usage/limit reports with their actual account and units.
6. Run a direct native harness concurrently and after proxy shutdown.
7. Remove only experiment-owned sessions, transcripts, credentials and files.

Pass: record observed behavior and limits, including failures and untested modes.
An HTTP response, help text, process start or chat answer alone does not establish
tool fidelity, native-config preservation or successful conversation resume.

## Follow-on decision

Only after these gates should a bounded implementation proposal select direct
connection profiles, a managed LiteLLM sidecar or native-harness selection.
Keep all existing harnesses available regardless of that decision.

Personal-account pooling, arbitrary executable plugins, automatic cross-provider
fallback, internet access and WSL proxy connectivity require separate approval.
WSL work must not widen a loopback listener to the LAN.

## Verification of this documentation

For this documentation-only change: check relative links, source traceability,
proposal/verification labels, LF/ASCII in new files and the scoped Git diff.
No application build or live-account test is required to establish that these
documents exist. Such checks would not prove the unimplemented design works.
