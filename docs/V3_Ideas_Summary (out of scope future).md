# TauShell V3 Ideas Summary

## Overview

TauShell V3 evolves from a terminal session manager into a complete AI Workspace Orchestrator.

## Major Architectural Themes

### 1. Workspace-Centric Design

Instead of managing individual terminal windows, users manage workspaces.

Example:

```yaml
workspace: SaaS-App

sessions:
  - architect
  - implementer
  - reviewer
  - researcher
```

A workspace can be restored as a single unit after reboot.

---

### 2. AI Fleet Restoration

After system restart:

- Restore all configured sessions
- Re-launch PowerShell environments
- Re-inject session variables
- Resume supported AI harnesses
- Reconnect mobile monitoring

Goal:

```text
One-click restore of the entire AI workspace.
```

---

### 3. Secret Vault

Introduce encrypted secret storage.

Example:

```yaml
environment:
  ANTHROPIC_API_KEY:
    secret: anthropic-prod

  OPENAI_API_KEY:
    secret: openai-prod
```

Benefits:

- No global Windows environment pollution
- No plaintext API keys in configuration files
- Session-specific credentials

---

### 4. Tool Action Abstraction

TauShell exposes tool-independent actions.

Example UI:

```text
[Opus]
[Sonnet]
[Compact]
```

Underlying plugins translate actions into harness-specific commands.

Example:

```text
/model opus
```

This avoids hard-coding UI to a specific AI tool.

---

### 5. Terminal Compatibility Layer

Centralise terminal configuration and fixes.

Managed features:

- UTF-8 defaults
- ANSI colour support
- Nerd Font validation
- Emoji rendering
- Box drawing characters
- PowerShell bootstrap scripts

Goal:

Every session starts in a known-good environment.

---

### 6. Agent State Machine

Move beyond simple Running/Stopped state.

Proposed states:

```text
Starting
Working
Thinking
Executing
Waiting For Input
Waiting For Approval
Needs Attention
Restoring
Disconnected
Failed
```

These states power smarter notifications.

---

### 7. Rich Mobile Companion

Capabilities:

- Session overview
- Live terminal view
- Prompt submission
- Action buttons
- Notifications
- Workspace restoration

No remote desktop required.

---

### 8. Intelligent Attention Detection

Combine multiple signals:

- Screen stability
- Output silence
- Cursor position
- Plugin rules
- OSC terminal events

Goal:

Detect when a human should return to the session.

---

### 9. Project-Aware Workspaces

Organise sessions around repositories.

```text
Project A
 ├─ Claude Architect
 ├─ Copilot Reviewer
 └─ Gemini Researcher
```

Rather than organising around terminal tabs.

---

### 10. Future Vision

TauShell becomes:

```text
Kubernetes for local AI coding agents
```

Managing:

- Agent lifecycle
- Restoration
- Configuration
- Monitoring
- Notifications
- Remote interaction

while remaining compatible with standard PowerShell-based workflows.

## Recommended V3 Deliverables

- Full technical specification
- Plugin SDK specification
- SQLite database schema
- REST API specification
- WebSocket protocol specification
- Security architecture
- Mobile UI wireframes
- ConPTY implementation design
- Deployment and packaging strategy
- Development milestone plan
