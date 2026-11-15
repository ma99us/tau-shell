# Provider gateway study

Status: proposed, unimplemented feasibility study. Documentation approval does
not approve a proxy implementation, account integration, or paid experiments.
V2 and the Phase 1 boundaries in [AGENTS.md](../../AGENTS.md) remain authoritative.

## Verdict

TauShell can plausibly offer one provider-selection surface and launch harnesses
with connection overrides **without changing their global defaults**. A local
LiteLLM gateway is an option, not a prerequisite for every connection.

That does not establish a universally supported way to use Claude Code with
every model and every subscription:

- Corporate LiteLLM serving Claude is the lowest-risk starting point.
- LiteLLM documents Copilot integration, including Anthropic Messages support,
  but third-party documentation is not vendor or corporate approval.
- Anthropic explicitly does not support Claude Code with non-Claude models
  through gateways.
- OpenAI API billing and ChatGPT/Codex subscription access are different.
- Available models and remaining allowances cannot be reliably inferred from
  installed harnesses or normalized into one "tokens left" number.

Sources, qualifications and unresolved gates are in the
[feasibility assessment](01-feasibility.md).

## Required behavior

The preferred design retains existing harness settings, skills, hooks, MCP
servers, permissions, plugins, logins and conversation stores. Only the opted-in
launch's provider connection changes. Direct launches outside TauShell retain
their normal configuration and account.

Show eligible providers together, but identify the account and billing owner
before the user confirms a route. Do not silently switch provider, personal/
corporate account, or subscription/API billing. Sending a conversation to a new
provider is a data-disclosure decision, not just changing a model name.

## Read next

1. [Feasibility and provider constraints](01-feasibility.md): supported versus
   merely translatable routes, subscriptions, quotas and source evidence.
2. [TauShell integration design](02-taushell-design.md): non-invasive launch
   configuration, optional sidecar, secrets, policy and current integration seams.
3. [Validation and decision gates](03-validation-plan.md): staged experiments,
   acceptance criteria and explicit unverified behavior.

## Scope and evidence

The initial candidate is optional, local Windows/PowerShell use, focused on
corporate accounts. Personal profiles, WSL gateway access, automatic failover,
internet access and a general executable-plugin framework are deferred.
Existing WSL and native harness behavior must remain unchanged.

Research snapshot: 2026-11-15; repository sampled at `5af57dc`, with a clean
worktree before these documents. Evidence consists of selected repository source
and public vendor documentation. No credentials, account state or private
endpoints were inspected; no gateway was installed or live model requests made.
This is not a full architecture, security, dependency or performance audit.

Open decisions include corporate authorization for Copilot gateway use, the
actual OpenAI access type, secure per-launch credential precedence, exact
provider/model/version compatibility and the minimal Windows gateway package.
These documents record a proposal, not successful runtime verification.
