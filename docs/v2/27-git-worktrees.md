# Git worktrees

Status: implemented in the working tree; verified only with unit tests and
disposable temporary git repositories. Not yet exercised in a real desktop window,
with a real harness, or in WSL. See [Not verified](#not-verified).

A session can work in its own git worktree: a separate folder and branch beside
the main checkout, so parallel sessions in one repository cannot overwrite each
other's files. It is opt-in per session. Nothing about worktrees is required to
use TauShell.

## User flows

- **Start new session:** the "Work in a new git worktree" toggle. It shows the
  worktree name (default: the project folder name), the new branch
  (`taushell/<name>`, editable), a "Start from" branch picker (default: the
  main checkout's current branch) and the folder that will be created.
  A dirty main checkout shows a warning: uncommitted changes are not carried over.
- **Clone:** a plain clone shares the original's folder. "Clone into a new git
  worktree" creates a new worktree in the original's repository and points the
  clone at it. The runtime stays inherited and frozen, like the folder.
- **Existing worktree:** the dialog lists the repository's other worktrees; picking
  one only fills the folder. The session is not marked as TauShell-created, so
  TauShell never offers to remove it.
- **Top bar:** in a linked worktree the git chip shows a worktree icon and the
  worktree name in place of the branch. The tooltip shows the branch, the
  worktree folder and the main repository. This applies to every linked
  worktree, including ones TauShell did not create. Home cards show a small
  `worktree` marker for sessions TauShell created a worktree for.
- **Return path:** the existing **Sync changes** button commits, rebases and
  pushes. A new branch has no upstream, so Sync publishes it with
  `git push -u <first remote> <branch>`. An **Open pull request** button (shown
  in worktrees) opens the GitHub compare page for the branch. The host builds the
  address from the `origin` remote; only github.com remotes are supported and the
  frontend never supplies a URL.
- **Delete:** the delete dialog offers "Also remove the worktree folder" (on by
  default only when the worktree is clean) and "Also delete the branch" (off by
  default). Uncommitted changes, or unpushed commits when deleting the branch,
  block removal until the user ticks "Discard that work and remove anyway".
- **Workspaces:** a linked worktree groups with its main repository on Home and
  in the sidebar (read from the worktree's `.git` file, no git process).
  Submodules stay their own group.

## Design

- `internal/worktree` inspects, lists, creates and removes worktrees. It runs git
  through `presets.RunGit`, so it works natively or inside the session's WSL
  distro (`presets.WithWSL`). Worktree folders for WSL are Linux paths.
- Location: `<parent of main checkout>\<repo>.worktrees\<name>`. Names are
  1-48 lowercase letters, digits, dots, dashes or underscores; Windows reserved
  names are refused. Branch names are checked with `git check-ref-format`.
- `.worktreeinclude` in the main checkout lists gitignore-style patterns of
  files git ignores (for example `.env`); they are copied into a new worktree.
  Only ignored, untracked regular files are copied and nothing is overwritten.
  Copy problems are warnings in the diagnostics log, not failures.
- A new worktree is created from the chosen base (default: current HEAD of the
  main checkout). A repository with no commits is refused.
- The session YAML records `worktree: {name, branch, base, repo}` only when
  TauShell created the folder (see the [YAML schema](18-yaml-schema.md)). That
  block is the only thing that permits removal. If saving the session fails, the
  new worktree is removed again.
- Removal ([`App.DeleteSessionWithWorktree`](../../cmd/taushell/worktree_windows.go))
  re-checks the folder is still the recorded linked worktree, refuses when
  another session uses the same folder, runs the session delete (which refuses a
  running session) and then `git worktree remove`, `git worktree prune` and, if
  asked, `git branch -d` (`-D` when forced). The main checkout is never removed.
  If the folder removal fails after the session was deleted, the error says so.
- Status: `GetGitStatus` adds the worktree info, cached per folder for a minute.

## Not verified

- No real-window check of the dialog, top bar, delete dialog or Open PR.
- Harness launch in a linked worktree was exercised with
  `TAUSHELL_LIVE_WORKTREE=1 TAUSHELL_LIVE=gsd|opencode|codex go test ./internal/sessionhost -run TestLiveHarness -v`:
  all three start and stop cleanly in the worktree folder. Model replies could
  not be checked because the development machine had no usable model access;
  the same runs fail the same way without a worktree.
- WSL worktree creation, `.worktreeinclude` copy and removal are written but
  untested.
- Open PR needs a github.com `origin`; other hosts show an error.
- Not built: a "Ship via PR" skill action, merging into the base branch locally,
  diff review, automatic cleanup, snapshots before delete, a setup script.
- Windows path length: paths over 200 characters produce a warning only.

Earlier [V3 planning](../v3/03-parked-and-rejected.md) rejected worktrees; this
feature supersedes that for the opt-in, per-session case described here.
