# Codex usage and toolbar handoff

**Status:** Local quota reader and static toolbar suggestions implemented; live
Codex compatibility and model availability are not verified.
**Updated:** 2026-11-08

This handoff records Codex quota and toolbar behavior, remaining verification,
and future model-catalog work. It does not authorize reading credentials
directly, changing Codex configuration, or using any quota-reset operation.

## Current TauShell behavior

- Home has a Copilot quota chip backed by `internal/quota` and
  `gh api /copilot_internal/user`. It runs through the user's GitHub CLI
  authentication, caches for five minutes, polls every five minutes, and is
  independently disabled with `quota.copilot` in `~/.tau_shell/config.yaml`.
  This endpoint is undocumented and already has a documented risk of change.
- Codex token counts are already ingested from rollout logs by
  `internal/usage`. Home's monthly usage summary includes Codex and the
  per-session UI estimates its token cost from model prices. These are
  retrospective token/cost totals, not the live allowance or reset windows
  imposed by a ChatGPT plan.
- Home has a separate Codex chip backed by a hidden local `codex app-server`
  process and `account/rateLimits/read`. It requests one snapshot, caches it for
  five minutes, and is independently disabled with `quota.codex` in
  `~/.tau_shell/config.yaml`. Missing or unsupported responses hide the chip.
  The live authentication and compatibility path has not been verified.

## Findings as of 2026-11-07

### ChatGPT plan used by Codex CLI

OpenAI's public user guidance tells users to check Settings or the usage
dashboard, and to enter `/status` in an active Codex CLI session. I found no
documented REST endpoint in the public OpenAI API reference for a personal
ChatGPT plan's live Codex allowance.

The official OpenAI Codex repository currently defines an app-server JSON-RPC
read method, `account/rateLimits/read`. Its response schema includes
`rateLimitsByLimitId`, rate-limit windows, used percentages, reset timestamps,
plan type, and optional credits/reset-credit information. The protocol also
defines `account/rateLimits/updated` notifications; these can be sparse, so a
client must merge them into the last full snapshot rather than replace it.
See the [Codex app-server account protocol](https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/src/protocol/v2/account.rs).

This is a local Codex app-server protocol, not a documented stable public REST
quota API. OpenAI's app-server integration guide documents stdio JSON messages
and OAuth access tokens for third-party integrations, but does not promise
that an external client can reuse a user's existing CLI login in every CLI
version. TauShell has not yet verified that authentication path. See the
[OpenAI Codex app-server guide](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server).

Window positions are not semantic: do not assume `primary` is always five
hours or `secondary` is always weekly. Use each window's reported duration to
label it (observed values include 300 minutes for five hours and 10,080 minutes
for weekly); omit unknown or unavailable windows rather than guessing. The
protocol can return multiple limit IDs, so identify the Codex allowance by
its supplied limit ID/name and treat unfamiliar buckets as unsupported until
their meaning is known.

### OpenAI API-key usage

OpenAI documents organization endpoints such as
`GET /v1/organization/usage/completions` and `GET /v1/organization/costs`.
They use an organization admin API key and report platform API usage/costs at
organization, project, user, API-key, or line-item aggregation levels. They
do not report the ChatGPT-plan allowance used by Codex CLI. They are not a
replacement source for the proposed plan quota chip. See the [organization
usage API](https://developers.openai.com/api/reference/cli/resources/admin/subresources/organization/subresources/usage)
and [organization costs API](https://developers.openai.com/api/reference/resources/admin/subresources/organization/subresources/usage/methods/costs).

OpenAI's Help Center also notes Codex activity is available in the Compliance
API for eligible workspaces. That is an audit/log surface, not the live
personal rate-limit snapshot needed for this widget.

## Implementation and verification

The local reader is implemented, but has not been live-verified against the
supported Codex CLI range (currently 0.166.0 through before 0.167.0). It starts
a hidden `codex app-server --stdio` process from PATH with a 15-second timeout,
initializes stdio JSON-RPC, and requests only `account/rateLimits/read`. It
does not pass auth tokens to TauShell or submit a model turn. Unsupported
protocol responses fail closed.

The parser renders only supplied rate-limit windows, labels each by its
reported duration, and includes a reset time only when present. The Home chip
shows remaining percentage, not a projection. The reader has a separate
five-minute cache and `quota.codex` opt-out; failures hide the chip and produce
one generic diagnostic per failure episode. Token/cost estimates remain a
separate TauShell usage summary.

Before claiming compatibility, run an opt-in read-only probe on Windows using
the installed supported CLI and compare the result with `/status` for the same
account. Confirm existing ChatGPT login works without TauShell opening,
copying, storing, or logging an auth token. The probe must not edit Codex
files, call account login or logout, or redeem/reset credits. Confirm that it
does not modify Codex config or conversation/session files; authentication
refresh, if needed, must remain inside Codex's own supported auth flow. Never
print response bodies or raw authentication data.

Do not add the organization API endpoints to the personal Codex quota path.
They require admin credentials and expose organization API billing data. Do
not call any app-server reset-credit consumption method for a display widget.

## Acceptance checks

- A ChatGPT-signed-in Codex account returns a read-only snapshot that matches
  the available windows shown by Codex `/status` or the official usage page.
- A weekly-only, five-hour-only, multi-limit, missing-reset, and unavailable
  response are displayed without relying on primary/secondary positions.
- Signed-out and API-key-only configurations hide the widget without a
  credential prompt or repeated diagnostic noise.
- The poll does not submit a Codex turn or modify Codex config/conversation
  files, and TauShell does not read, copy, persist, or log raw credentials or
  response data.
- Unit fixtures cover protocol versions and malformed/partial responses;
  any live probe remains opt-in and reports no account secrets.

## Codex session toolbar: model selection and commands (planned)

### Current behavior

The Codex manifest exposes `/compact` and `/status` as command buttons. It
advertises model selection through `--model`, so the Model menu can open
Codex's native `/model` picker, accept a custom model ID, and show static
suggestions from the OpenAI collection in LiteLLM. Catalog membership does not
prove account entitlement. The current model reader can observe the model
recorded in Codex rollout `turn_context`; this is useful for confirming a
selection after a turn but is not a model availability source.

### Model selection plan

Prefer a dynamic catalog tied to the active Codex account over a hard-coded
list that can become stale. The app-server protocol has a `model/list` method.
OpenAI's integration guidance notes that the returned catalog can be bundled
or cached depending on provider configuration, so a listed model must not be
treated as proof of plan entitlement. The OAuth `/v1/models` catalog described
for ChatGPT integrations provides display names and slugs filtered for
visibility, but it too is a catalog rather than an entitlement guarantee.
See the [Codex app-server guide](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)
and [models and inference guide](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference).

Implementation should preserve the native `/model` picker and custom model ID
entry as reliable fallbacks. If a supported dynamic catalog is available,
show its display name and exact slug, refresh it on account changes where the
protocol exposes that signal, and submit the selected slug using Codex's
supported interface. Then report the selection as confirmed only after
TauShell observes the new model in rollout metadata; otherwise say that the
selection was requested but could not be confirmed. Do not cache account
catalogs across account changes or present catalog membership as a promise
that the account can use a model.

Quota and model-catalog reads may share a future app-server connection if the
protocol supports it, but they must fail independently: an unavailable model
catalog must not hide quota data, and a quota failure must not remove the
native model picker. Do not add a second long-lived app-server process if one
reader can serve both safely.

### Useful command actions plan

Keep `/status`, `/compact`, the native `/model` picker, and single-Esc
interruption as the verified baseline for the current supported Codex range.
Probe additional high-value, low-risk actions against that range and add them
only after verifying their actual effect, not merely that Codex accepts the
input. `/new` remains unexposed because the existing live probe did not prove
that it starts a distinct conversation; `/help` and other candidate commands
also need verification before promotion.

Keep actions data-driven through manifest widgets or narrowly typed capability
metadata rather than Codex-specific UI branches. Preserve the existing
terminal-send behavior: commands must not overwrite an unsent draft, and an
action that submits input must make that submission behavior clear. Do not
offer actions that change approval or sandbox policy through a quick button.
Any future action that resets or abandons conversation state needs an explicit
confirmation and verified semantics.

### Acceptance checks

- Exercise each proposed command on the installed supported Codex version in a
  disposable project and account profile; verify its semantic result and
  failure behavior. Keep `/new` hidden unless a distinct new conversation is
  proven.
- Verify dynamic model names/slugs against Codex's native picker, test an
  unavailable model, and confirm a successful switch through subsequent
  rollout metadata. Keep custom IDs and the native picker available when the
  catalog is absent or unsupported.
- Test account/catalog refresh behavior and ensure catalog results are never
  described as plan entitlement. Never read or log raw auth tokens.
- Cover static action metadata and dynamic catalog mapping with fixtures;
  keep real Codex checks opt-in and within the manifest-tested version range.
- Confirm toolbar actions preserve drafts and do not accidentally submit a
  user's existing unsent input.

## References

- [Using Codex with your ChatGPT plan](https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan)
- [Codex app-server protocol account types](https://github.com/openai/codex/blob/main/codex-rs/app-server-protocol/src/protocol/v2/account.rs)
- [Codex rate-limit window observations](https://github.com/openai/codex/discussions/56932)
- [OpenAI Codex app-server integration guide](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)
- [OpenAI Codex models and inference guide](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
- [OpenAI organization usage API](https://developers.openai.com/api/reference/cli/resources/admin/subresources/organization/subresources/usage)
- [OpenAI organization costs API](https://developers.openai.com/api/reference/resources/admin/subresources/organization/subresources/usage/methods/costs)
