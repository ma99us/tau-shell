# Session Restoration

> **Historical draft; superseded for the current UI by
> [08-UI-Design.md](./08-UI-Design.md).** Startup restores the previous tab
> layout and selection only; it does not recreate or resume harness processes.
> Starting or restoring a harness requires an explicit user action. See also
> [docs/v2/04-session-restoration.md](../v2/04-session-restoration.md).

## Stored State

- Session definition
- Working directory
- Last known tool
- Snapshot metadata
- Resume strategy

## Restore Flow

1. Startup
2. Load persisted workspace
3. Recreate ConPTY sessions
4. Start PowerShell bootstrap
5. Launch AI tool
6. Execute resume command
7. Validate session health
