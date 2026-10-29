# 05 - Project File Drawer

Status: **future planning**. Rank 5. Sources: user decision; Orca file explorer and quick open.

## Problem

While working with an agent, the user often needs to find a file, read a Markdown plan or
spec, and refer to it in a prompt. Today that means switching to Explorer or an editor and
typing a path by hand.

## Guiding rule

Slick and simple. This is **not an IDE**: no editing, no tabs of code, no language features.

## User stories

- As a user, I want to browse the session's project folder, so that I can find files fast.
- As a user, I want to read a Markdown file in the app, so that I can check a plan or doc
  while the agent works.
- As a user, I want to copy a file's relative or full path, so that I can paste it into a
  prompt.
- As a user, I want to drop a file onto the terminal to paste its path, so that referencing
  files is one gesture.
- As a user, I want to open files from the command palette, so that I can jump by name.

## In scope

- A collapsible drawer for the active session showing its working directory as a tree.
- Respecting ignore rules for noise (for example `.git`, `node_modules`), with a toggle.
- Markdown viewer (read-only) with safe rendering and relative links and images.
- Context menu and keyboard actions: copy relative path, copy full path, reveal in
  Explorer, open with the default app.
- Drag a file onto the terminal to insert its path, quoted correctly for PowerShell.
- File names in the command palette quick open.
- Light git markers (see [06](06-status-insights.md)) are optional.

## Out of scope

- Editing, saving or creating files.
- Syntax-highlighted code viewing beyond plain text (candidate for later).
- Search inside files, diffs, git operations, other previews (PDF, images).

## Acceptance criteria

- The tree is limited to the session's working directory; paths outside it cannot be
  browsed, including through symlinks or `..`.
- Large folders stay responsive; the tree loads lazily.
- Markdown never runs scripts and does not load remote content without an explicit action.
- Copied relative paths are relative to the session working directory and use a consistent
  separator; full paths are absolute. Names with spaces are handled when pasted.
- Dropping a file pastes the path at the prompt without submitting.
- External links open only through the bound opener (known URLs); no `target="_blank"`.
- Works with keyboard only and does not steal focus from the terminal unexpectedly.

## Dependencies

- None blocking. Quick open shares the existing palette.

## Risks

- Drag and drop in WebView2 provides limited path information; a native bridge may be needed.
- Rendering untrusted Markdown in the app is an injection surface.
- Pasting into harnesses that treat paths specially (for example `@file` references) may
  differ per harness.

## Spikes

- WebView2 file-drop behaviour and obtaining full paths.
- Which Markdown renderer and sanitizer fit the existing bundle.
- Per-harness path syntax for file references (plain path versus `@path`).

## Open questions

- Should the drawer be per session or global?
- Show only the working directory, or allow pinned extra folders?
- Insert paths with or without a leading `@` for harnesses that support it?

## Docs to update when implemented

`08-UI-Design.md`, README, `11-security-model.md` (file access boundary, Markdown rendering).
