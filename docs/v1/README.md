# V1 Documentation Notes

The older documents in this folder are planning artifacts from an earlier
TauShell direction. They are useful historical context, but they do not replace
the active product and engineering specifications in [`../v2/`](../v2/).
Apply the currently agreed Phase 1 scope and the explicit decisions in the
current UI documents below where older text differs.

- [UI design](./08-UI-Design.md) records the agreed Windows desktop GUI:
  startup/setup screen, tool discovery, tabbed embedded terminals, background
  sessions, and explicit lifecycle choices.
- [UI implementation plan](./09-UI-Implementation-Plan.md) stages a Go-hosted
  WebView2 desktop app and embedded terminal feasibility prototype. The
  existing Bubble Tea TUI is disposable and must be replaced/removed; only
  the `list`, `start`, and `restore` CLI commands remain as CLI surfaces.

Some older V1 material contains proposals that are not current Phase 1
requirements:

| Older material | Current interpretation |
|---|---|
| [Configuration](./03-Configuration.md) includes `auto_restore`, startup commands, and secret interpolation in YAML. | Use the active [YAML schema](../v2/18-yaml-schema.md) and [environment guidance](../v2/10-environment-management.md). Restore is explicit; secret values do not belong in session YAML. |
| [Session restore](./05-Session-Restore.md) starts restoring the persisted workspace on app startup. | The target UI opens Home, where prior tab metadata is available without starting harnesses. Starting/restoring each harness requires an explicit user action. See [UI design](./08-UI-Design.md) and [session restoration](../v2/04-session-restoration.md). |
| [MVP roadmap](./06-MVP-Roadmap.md) places notifications and a mobile dashboard in Phase 1. | Phase 1 is a local Windows desktop application. Remote access, mobile, remote web dashboards, and notifications are out of scope. |
| [Plugin system](./04-Plugin-System.md) sketches per-tool YAML launch/resume/action command definitions. | Harness launch/resume behavior belongs behind tool-specific adapters. A single editable `launch_args` list is planned for user-supplied arguments; see [plugin system](../v2/05-plugin-system.md). |

The historical documents have not been comprehensively rewritten. Consult the
linked V2 documents for current product and engineering requirements.
