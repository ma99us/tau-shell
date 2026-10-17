# Architecture

## Core Components

- Session Manager
- ConPTY Host
- PowerShell Bootstrap Layer
- Tool Plugin Manager
- Attention Engine
- Snapshot Engine
- Restore Engine
- Notification Service
- Mobile Web UI

## Runtime Stack

TauShell -> ConPTY -> pwsh.exe -> AI Tool

The AI tool continues believing it runs inside a normal PowerShell session.
