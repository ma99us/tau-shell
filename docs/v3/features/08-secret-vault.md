# 08 - Secret Vault

Status: **future planning**. Rank 8. Sources: V3 ideas #3.

## Problem

Session `environment:` values are plaintext in YAML, which users hand-edit, copy and may
commit. They are also visible on the process command line today
([finding 2.1](../02-cross-cutting.md#21-environment-values-are-visible-on-the-process-command-line-phase-1-bug)).
Many harnesses sign in with OAuth, so the vault mainly helps API-key flows, provider
overrides and tool tokens that a user wants scoped to one session.

## User stories

- As a user, I want to store a secret once and reference it by name in a session, so that no
  plaintext key sits in YAML.
- As a user, I want a secret to apply only to the sessions that reference it, so that I do
  not pollute global Windows environment variables.
- As a user, I want to rotate or remove a secret in one place.

## In scope

- YAML reference syntax such as `secret: name` (V3 ideas example) as an alternative to a
  literal value.
- Storage in the OS secret store (Windows Credential Manager or DPAPI-protected data).
- A small UI to add, update, delete and list secret **names** (never show values by
  default).
- Injection into the session's process environment only, with no values in argv, YAML, logs,
  diagnostics or the renderer.
- Clear failure when a referenced secret is missing; the session does not start.

## Out of scope

- Syncing secrets across machines; sharing; team vaults.
- Provider OAuth token management.
- Importing from third-party password managers (later).

## Acceptance criteria

- No secret value appears in session YAML, `state.db`, diagnostics, the command line of any
  child process, toasts or bridge payloads.
- A session that references a missing secret fails to start with a message naming the
  secret, not its value.
- Cloning a session copies the reference, never the value.
- Deleting a secret warns which sessions reference it.
- The vault is accessible only to the current Windows user.
- The Phase 1 command-line leak fix is in place before this ships.

## Dependencies

- **Hard dependency:** the Phase 1 fix for environment values on the command line.
- Windows secret store; portability seam ([section 3](../02-cross-cutting.md#3-portability-track-not-ranked)).

## Risks

- Anything running as the same user and able to read the child environment
  (for example via process inspection tools) can still read injected values. State this
  honestly in the security model.
- Child processes of the harness inherit the variable.
- A false sense of security if values still appear in terminal echo or logs.

## Spikes

- Credential Manager versus DPAPI file: size limits, listing, backup behaviour.
- Passing environment through ConPTY without placing values in argv.

## Open questions

- Should secrets be shareable between sessions by default?
- Is a master-password step wanted, or rely on the Windows login?

## Docs to update when implemented

`10-environment-management.md`, `11-security-model.md`, `18-yaml-schema.md`, README.
