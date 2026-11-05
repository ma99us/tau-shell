import type { GitStatus } from "./bridge";

// Ahead/behind are -1 when the branch has no upstream; nothing is invented then.
export function gitLabel(git: GitStatus): string {
  const { branch, dirty, ahead, behind } = gitParts(git);
  const counts = [ahead > 0 ? `+${ahead}` : "", behind > 0 ? `-${behind}` : ""].filter(Boolean).join(" ");
  return `${branch}${dirty ? "*" : ""}${counts ? ` ${counts}` : ""}`;
}

// Pieces of the chip: the view marks dirty with "*" and shows an up arrow for
// unpushed commits (ahead) and a down arrow for commits not yet pulled (behind).
export function gitParts(git: GitStatus): { branch: string; dirty: boolean; ahead: number; behind: number } {
  return { branch: git.branch, dirty: git.dirty, ahead: Math.max(git.ahead, 0), behind: Math.max(git.behind, 0) };
}

export function gitTooltip(git: GitStatus): string {
  const parts = [git.dirty ? "Uncommitted changes (* after the branch)" : "Working tree clean"];
  if (git.ahead < 0) parts.push("No upstream branch to compare with");
  else parts.push(`${git.ahead} unpushed commit${git.ahead === 1 ? "" : "s"} (up arrow), ${git.behind} not yet pulled (down arrow)`);
  return parts.join(". ");
}
