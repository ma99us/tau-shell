import type { GitStatus } from "./bridge";

// Ahead/behind are -1 when the branch has no upstream; nothing is invented then.
export function gitLabel(git: GitStatus): string {
  const { branch, dirty, counts, unsynced } = gitParts(git);
  return `${branch}${dirty ? "*" : ""}${counts ? ` ${counts}` : ""}${unsynced ? "*" : ""}`;
}

// Pieces of the chip so the view can highlight the "*" markers: dirty after the
// branch, unsynced (unpushed or missing commits) after the counts.
export function gitParts(git: GitStatus): { branch: string; dirty: boolean; counts: string; unsynced: boolean } {
  const counts = [git.ahead > 0 ? `+${git.ahead}` : "", git.behind > 0 ? `-${git.behind}` : ""].filter(Boolean).join(" ");
  return { branch: git.branch, dirty: git.dirty, counts, unsynced: counts !== "" };
}

export function gitTooltip(git: GitStatus): string {
  const parts = [git.dirty ? "Uncommitted changes (* after the branch = dirty)" : "Working tree clean"];
  if (git.ahead < 0) parts.push("No upstream branch to compare with");
  else parts.push(`${git.ahead} ahead, ${git.behind} behind upstream${git.ahead > 0 || git.behind > 0 ? " (* after the counts = not in sync)" : ""}`);
  return parts.join(". ");
}
