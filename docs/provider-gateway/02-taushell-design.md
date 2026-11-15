# Proposed TauShell integration

Status: design proposal, not implemented. See [feasibility](01-feasibility.md)
for provider support and [validation](03-validation-plan.md) for proof gates.

## Non-invasive launch is the primary requirement

An opted-in session may change its provider connection, not the user's default
harness setup. Keep existing skills, hooks, MCP servers, plugins, permission
controls, logins and conversation stores. Direct launches outside TauShell,
including concurrent ones, must retain their normal connection and defaults.

Never configure this through global/project settings edits, shell-profile edits,
registry environment changes, `gh auth switch`, or login/logout operations.
Backup/edit/launch/restore is also rejected: crashes and concurrent launches
make temporary global edits unsafe.

Use documented launch flags and child-only environment overrides. Store only
non-secret overlays under TauShell-owned storage. Do not mutate the host process
environment to select one session's route. Ordinary harness writes to history
and logs are still possible with shared native homes; preserving defaults does
not mean the harness writes no runtime state.

## Existing integration seams

These are sampled observations at `5af57dc`, not defects or approved changes:

| ID | Evidence and implication | Confidence; impact; future change risk |
| --- | --- | --- |
| ARCH-001 | [Harness documentation](../v2/05-plugin-system.md) and [Adapter](../../internal/harness/adapter.go) describe declarative harness/lifecycle integration, not executable service plugins. A gateway belongs behind a separate optional service boundary. | 95, directly inspected contract; HIGH; MEDIUM |
| ARCH-002 | [GH quota runner](../../internal/quota/gh_windows.go) uses the active `gh` login and an internal endpoint; [LiteLLM resolution](../../internal/quota/litellm.go) reads default Claude settings/env; [bridge caching](../../cmd/taushell/quota_windows.go) lasts five minutes. These chips are not account-pinned routing authorization. | 95, direct source evidence; HIGH; MEDIUM |
| ARCH-003 | [Session configuration](../../internal/session/config.go) already carries environment and secret references; [launch management](../../internal/sessionhost/manager_windows.go) resolves them before launch; [ConPTY](../../internal/conpty/process_windows.go) merges a child environment. Reuse these boundaries. [Vault](../../internal/vault/vault.go) currently describes secrets leaving only for child environments; direct host proxy use needs a contract review. | 95, direct source evidence; HIGH; MEDIUM |
| ARCH-004 | [Claude config-root lookup](../../internal/harness/home_session.go) and [Codex session lookup](../../internal/harness/gsd_codex.go) consult host environment. Redirecting only a child's config home can disconnect discovery/resume/model/usage readers from that child's state. | 95, direct source evidence; HIGH; MEDIUM |

All findings are proposed design observations, not implementation authorization.
Preserve harness-neutral core lifecycle, adapter-owned argv/resume, existing
skills, prompt-queue guards and cumulative usage. Do not claim cross-harness
resume. A future executor must revalidate these observations.

## Per-harness configuration strategy

Documented mechanisms below are candidates, not verified secure launch recipes.
Probe the installed version rather than assuming current website behavior.

| Harness | Preferred candidate | Precedence and compatibility gate |
| --- | --- | --- |
| Claude Code | Minimal `--settings` overlay for non-secret route/model settings; child environment or a documented credential helper for secrets | Settings `env` can override inherited environment. Command-line settings outrank ordinary user/project/local settings, not managed policy. Verify secure credential precedence and background-agent behavior. |
| Copilot CLI | `COPILOT_PROVIDERS_CONFIG` for a TauShell-owned non-secret provider definition and explicit `--model`; supported launch-scoped auth where needed | Provider files can contain plaintext keys; do not use that mode. Verify a supported secret-reference mechanism, auth precedence and exact BYOK behavior. BYOK consumes upstream API access, not Copilot subscription allowance. |
| Codex | Per-run `-c`/`--config` provider/base-URL overrides, with secret values supplied through verified environment references | Verify installed-version provider schema, Responses support and auth selection. Never put secrets in `-c` or forward native subscription credentials to an unintended endpoint. |

### Claude Code details

The [CLI reference](https://code.claude.com/docs/en/cli-reference) documents
`--settings` as a file or inline JSON. The
[settings documentation](https://code.claude.com/docs/en/settings) says omitted
keys retain file-based values and managed keys cannot be overridden. This is a
better fit than suppressing user settings wholesale.

However, [environment documentation](https://code.claude.com/docs/en/env-vars)
and [gateway connection guidance](https://code.claude.com/docs/en/llm-gateway-connect.md)
say settings-file `env` entries can replace inherited shell values. Thus setting
`ANTHROPIC_BASE_URL` and a credential in the child environment alone is not
sufficient when existing settings supply conflicting values.

Set a gateway credential as well as a base URL: Anthropic says a base URL alone
can leave a saved subscription login active. Never place that credential in
inline `--settings`, persistent overlay files or argv. Evaluate the documented
`apiKeyHelper` route or effective child-secret injection without weakening
secret handling. Helper transport, competing credential variables, interactive
auth prompts and their precedence remain test gates, not a finished recipe.
If there is no safe effective override, block the opted-in launch with an
actionable explanation rather than changing defaults or using another account.

The gateway guidance also warns that shell-only variables may not reliably
reach supervisor-hosted background agents. Verify those modes explicitly.
Do not persist secrets to make background execution work.

Do not use `--setting-sources` to strip existing functionality, or `--bare`,
safe/restricted modes merely to force a route. Honor managed policy; a managed
endpoint restriction may legitimately prevent the local gateway entirely.

### Keep native configuration homes

Do not change `CLAUDE_CONFIG_DIR`, `COPILOT_HOME` or `CODEX_HOME` by default.
They relocate more than connection settings:

- [Claude configuration/authentication](https://code.claude.com/docs/en/authentication)
  includes settings, plugins, history and login state.
- [Copilot's configuration directory](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)
  includes settings, skills, hooks, plugins and session state.
- [Codex state and configuration](https://developers.openai.com/codex/config-advanced)
  includes config, history and credential-cache behavior.

A separate home can hide native functionality and would need explicit context
propagation through every host-side reader. It is not the selected design.
Do not copy or symlink personal credential stores into a managed profile.
Keyring partitioning by config home was not established by this study.

For Copilot, consult the
[command reference](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference)
and [custom provider configuration](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-llm-models).
Environment-token precedence over stored login and secure provider-file secret
references still require verification. Existing native auth must not be changed.

## Connection profiles and recommendations

Propose one optional provider-profile surface, not a new harness type. A profile
identifies an approved upstream endpoint, account, payer, policy class, exact
model deployments, compatible harnesses, credential references and capabilities.
No new YAML/API schema is committed by this study.

Persist human-selected profile intent separately from changing health, quota,
timestamps and observed models. Runtime observations belong in runtime state,
not session YAML. Never return credential values to the renderer.

Recommendation order:

1. Apply organization policy, permitted account and approved egress constraints.
2. Check harness/protocol/model capabilities and verified credential identity.
3. Consider timestamped availability, budget and rate-limit evidence in its
   original units. Explain unknown or stale evidence.
4. Show the proposed provider, account, model and billing owner; require consent.
5. Pin the selected route. Do not silently reroute if a later request fails.

Do not pool personal and corporate credentials behind an ambiguous alias.
Even switching between two Claude deployments can disclose the existing
conversation to a different service. Warn before such a change; require explicit
approval and prefer a new conversation if safe continuation is unproven.

## Optional gateway lifecycle

Conceptually:

```text
TauShell profile selection -> per-launch connection overrides -> native harness
                                                            |
                               authenticated local endpoint (optional)
                                                            |
                                        pinned approved upstream + account
```

Prefer direct profile injection when it solves the need. If protocol unification
is justified, evaluate a supervised LiteLLM sidecar before writing a Go protocol
translator. No arbitrary third-party executable-plugin framework is needed.

Installation must be optional and confirmed, with source identification,
download verification, detected version and capability checks. Do not silently
pin, downgrade or update. Establish the minimal supported Windows runtime;
static LiteLLM config does not automatically require a database.

Before launching an opted-in harness, validate the route and start/check the
owned service. Reject port conflicts, failed readiness or unavailable named
credentials. Use a scoped local credential mapped to a fixed profile rather than
giving the harness an upstream key usable with arbitrary routes.

Keep the sidecar available while dependent sessions need it. Cancel upstream
requests when clients cancel; surface startup failures and crashes. On shutdown,
respect session confirmation and graceful termination before stopping the
dependent service. Do not terminate an externally managed proxy or another
user's process. A failure must not change any native harness default.

Overlays contain no secrets and use user-only access controls. Keep the resolved
non-secret profile/version context needed for restore; regenerate launch-specific
files and local credentials as necessary. Clean only owned artifacts once no
session uses them. Do not restore onto a different account automatically.

## Trust and failure boundaries

- Bind only authenticated Windows loopback; never use a wildcard/LAN listener.
  Loopback authentication is not isolation from malicious same-user processes.
- Validate local HTTP host/origin behavior; disallow arbitrary browser clients
  and unintended cross-origin access. Enforce request/concurrency bounds.
- Keep upstream credentials in an approved store/child environment, not argv,
  settings overlays, YAML, logs, URLs, runtime database or bridge responses.
- Use TLS verification, approved upstream hosts and verified corporate CA
  configuration. Reject credential-forwarding redirects and arbitrary targets.
- Do not log prompts, responses, tool contents or transcripts by default.
  Metadata/error redaction is part of the acceptance gate.
- Disable shared-login and cross-route fallback. Distinguish auth denial, policy
  denial, exhausted budget, throttling and network failure. Surface each.
- Do not replay partially streamed or tool-bearing requests to another provider.
  Any retries must preserve cancellation and avoid duplicate side effects.
- Distinguish recorded costs, estimates, unpriced usage, subscription billing
  units and local counters. Never interpret absent pricing as zero.

WSL gateway access is deferred: loopback reachability depends on network mode.
Do not expose a Windows proxy to the LAN to make WSL work. Preserve existing
WSL support and limitations.

These boundaries extend the existing
[security model](../v2/11-security-model.md) and
[environment rules](../v2/10-environment-management.md); they are not claims
that a gateway implementation has passed security or end-to-end testing.
