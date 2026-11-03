import type { GitStatus } from "./bridge";

// Ahead/behind are -1 when the branch has no upstream; nothing is invented then.
export function gitLabel(git: GitStatus): string {
  const counts = [git.ahead > 0 ? `+${git.ahead}` : "", git.behind > 0 ? `-${git.behind}` : ""].filter(Boolean).join(" ");
  return `${git.branch}${git.dirty ? "*" : ""}${counts ? ` ${counts}` : ""}`;
}

export function gitTooltip(git: GitStatus): string {
  const parts = [git.dirty ? "Uncommitted changes (* = dirty)" : "Working tree clean"];
  if (git.ahead < 0) parts.push("No upstream branch to compare with");
  else parts.push(`${git.ahead} ahead, ${git.behind} behind upstream`);
  return parts.join(". ");
}
