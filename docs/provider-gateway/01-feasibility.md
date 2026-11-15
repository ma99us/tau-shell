# Provider feasibility

Status: proposed, unimplemented study. See the [study scope](README.md).
Public sources below were retrieved during research on 2026-11-15 and can change.
Documented mechanisms are not proof that the combined TauShell route works.

## What is being unified?

| Concept | Responsibility |
| --- | --- |
| Harness | Agent loop, tools, permission prompts and conversation state |
| Provider/account | Remote service, authentication, policy boundary and payer |
| Model deployment | Exact model ID, capabilities, context limit and entitlement |
| Gateway | Fixed upstream routing, protocol transport and optional translation |
| Selector | Explaining eligible routes and recommending a user-confirmed choice |

A protocol translator does not convert an entitlement. Access to Copilot does
not imply an OpenAI API balance; a Codex subscription does not imply a general
API key. Nor does switching harnesses transfer resumable conversation state.

## Route matrix

| Route | Evidence and support level | Decision |
| --- | --- | --- |
| Claude Code -> corporate LiteLLM -> Claude | Anthropic documents compatible gateways; the exact corporate deployment is untested here [A1] | Preferred starting point after corporate approval |
| Claude Code -> local LiteLLM -> corporate LiteLLM -> Claude | Compatible protocol pattern, with an additional hop to verify [A1, L1] | Add only if local isolation/selection justifies it |
| Claude Code -> LiteLLM -> Copilot-hosted Claude | LiteLLM documents its Copilot provider and `/v1/messages`; no GitHub endorsement of this integration was established [L2, G1, G2] | Experimental; policy, billing and protocol gates first |
| Claude Code -> OpenAI or other non-Claude models | Translation is technically possible; Anthropic says this use is unsupported [A1, L1] | Not a vendor-supported Claude Code route |
| Compatible harness -> OpenAI API through a gateway | Standard usage-billed API access, separate from a subscription [O1, O2] | Assess if API access and spending are authorized |
| Native Codex -> ChatGPT subscription | OpenAI documents ChatGPT sign-in for subscription access [O1] | Preserve as the baseline |
| Gateway -> ChatGPT/Codex subscription backend | LiteLLM documents subscription integrations; OpenAI support for general API use of that backend was not established [L3, L4, O1] | Defer pending vendor/policy review |

### Claude Code is not a universal model frontend

Anthropic states: "Any gateway that exposes a supported API format works," but
also "doesn't support routing Claude Code to non-Claude models through any
gateway" [A1]. The first statement is not a guarantee of tool/reasoning parity
for an arbitrary translated model.

A gateway must handle `/v1/messages`, streaming and applicable beta headers.
Token counting, tool calls/results, thinking, context management, structured
output, prompt caching and usage reporting require separate checks [A2].
Claude Code sends Anthropic-format features even when the gateway translates
to another upstream. Merely returning text or HTTP 200 is insufficient.

Model aliases can also affect assumed context windows and capabilities.
Do not disguise a smaller or incompatible model behind a Claude-looking alias
and assume the client adapts. Pin and test actual deployment capabilities.
Include small/background-model requests, not just the main conversation.

### GitHub Copilot: separate the access paths

GitHub documents the Copilot SDK as a programmatic agent runtime [G1]. It is
not automatically a raw Messages endpoint for Claude Code. Using the SDK as an
agent is an alternative to, not an implementation of, transparent model routing.
Its BYOK support accepts upstream providers; this is the opposite direction
from spending Copilot entitlements through another harness.

LiteLLM documents a Copilot provider with OAuth and GitHub App paths [L2].
GitHub documents organization-enabled, short-lived App installation tokens for
Copilot SDK requests [G2]. The installation's organization pays, subject to its
model policies and spending controls [G3]. This is not equivalent to pooling
several existing personal or corporate user subscriptions.

The reviewed GitHub pages did not establish support for LiteLLM as that client.
Confirm the intended integration with the organization/vendor rather than
assuming that a working token makes every client authorized. LiteLLM also
documents shared-login fallback behavior when named credentials are absent;
such fallback is incompatible with this proposal's account-pinning requirement.

Current GitHub documentation describes AI-credit-based billing, with corporate
allowances pooled at the billing entity [G3, G4]. Some existing annual individual
plans retain legacy premium-request billing [G5]. Consequently "premium requests
left" is not a universal description of Copilot capacity.

GitHub Models is not a replacement source of Copilot entitlements: its
documentation says that separate service was retired on July 30, 2026 [G6].

### OpenAI: two branches until access is known

OpenAI distinguishes "Sign in with ChatGPT for subscription access" from
"Sign in with an API key for usage-based access" [O1]. Its Codex access-token
documentation directs general API calls to Platform API keys.

If API access exists, configure a compatible harness/provider with explicit
API spending approval. If only a subscription exists, retain native Codex as
the documented baseline. Do not extract its cached credentials for a proxy.
LiteLLM's ChatGPT backend integration and Codex forwarding tutorial are evidence
of third-party mechanisms, not a conclusion about OpenAI authorization [L3, L4].
Never silently fall back from subscription usage to separately billed API use.

## Quota-aware recommendations, not guaranteed capacity

| Signal | What it can establish | What it cannot establish |
| --- | --- | --- |
| Corporate LiteLLM budget | Reported member/team spend and reset when exposed | All model entitlements or remaining token/rate capacity |
| Copilot usage/billing report | Applicable allowance/spend for its billing scope | A universal documented remaining-capacity API or permission for another client |
| Codex local rate-limit snapshot | Reported percentage windows for the signed-in account | OpenAI API credit or exact tokens remaining |
| OpenAI API rate-limit data | Organization/project request or token-rate limits [O2] | A subscription balance or guaranteed success of the next request |
| Local gateway counters | Traffic observed by that gateway | Usage outside it or upstream entitlements |

Track account, source, unit, scope, timestamp and freshness with every observation.
Unknown, stale, unavailable, unlimited and exhausted are different states.
Budget, subscription allowance and RPM/TPM are different constraints.
An apparently healthy allowance does not exclude transient throttling.

Use current provider/account-specific evidence where available, and explain its
limits. Do not guess missing quotas, automatically consume overages, or present
gateway-recorded subscription cost of $0 as zero economic cost. Account identity
must match the route; an unrelated `gh` login is not sufficient.

## Options and recommendation

1. **Per-launch connection profiles first:** simplest way to preserve native
   harnesses without operating another service. It does not unify protocols.
2. **Optional LiteLLM sidecar:** useful when one endpoint or protocol translation
   is actually needed. Adds dependencies, credential/lifecycle handling and
   another failure boundary.
3. **Native harness selection:** retain Copilot for Copilot and Codex for its
   subscription when gateway support is unresolved. A unified TauShell picker
   can still explain these choices without pretending they are one backend.
4. **Custom Go translator or generic executable plugins:** not recommended for
   the first experiment; neither removes entitlement or compatibility limits.

Static LiteLLM configuration does not universally require PostgreSQL [L5].
Virtual keys and some dashboard/account flows do require a database [L2, L4, L6].
The exact smallest Copilot-enabled Windows deployment is unverified.

Proceed to a bounded experiment only after policy, billing ownership and secure
launch configuration are established. The [validation plan](03-validation-plan.md)
defines those gates. No legal approval or production-readiness claim is implied.

## Public sources

- [A1: Anthropic gateway support and limits](https://code.claude.com/docs/en/llm-gateway)
- [A2: Claude Code gateway protocol](https://code.claude.com/docs/en/llm-gateway-protocol.md)
- [L1: LiteLLM Anthropic-format ingress](https://docs.litellm.ai/docs/anthropic_unified)
- [L2: LiteLLM GitHub Copilot provider](https://docs.litellm.ai/docs/providers/github_copilot)
- [L3: LiteLLM ChatGPT provider](https://docs.litellm.ai/docs/providers/chatgpt)
- [L4: LiteLLM Codex subscription routing tutorial](https://docs.litellm.ai/docs/tutorials/openai_codex_byok)
- [L5: LiteLLM static proxy configuration](https://docs.litellm.ai/docs/proxy/configs)
- [L6: LiteLLM virtual keys](https://docs.litellm.ai/docs/proxy/virtual_keys)
- [G1: Copilot SDK quickstart](https://docs.github.com/en/copilot/get-started/sdk-quickstart)
- [G2: Copilot SDK server-to-server tokens](https://docs.github.com/en/copilot/how-tos/copilot-sdk/auth/server-to-server-tokens)
- [G3: Copilot organization billing](https://docs.github.com/en/copilot/concepts/billing-and-usage/organizations-and-enterprises/billing)
- [G4: Copilot individual billing](https://docs.github.com/en/copilot/concepts/billing-and-usage/individuals)
- [G5: Legacy premium-request billing](https://docs.github.com/en/copilot/reference/copilot-billing/request-based-billing-legacy/copilot-requests)
- [G6: GitHub Models retirement](https://docs.github.com/en/github-models)
- [O1: OpenAI Codex authentication](https://learn.chatgpt.com/docs/auth)
- [O2: OpenAI API rate limits](https://developers.openai.com/api/docs/guides/rate-limits)
