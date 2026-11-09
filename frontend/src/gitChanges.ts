import type { GitChange } from "./bridge";

export type ChangeNode = { name: string; path: string; dir: boolean; code?: string; staged?: boolean; children: ChangeNode[] };

const labels: Record<string, string> = { M: "Modified", A: "Added", D: "Deleted", R: "Renamed", U: "Untracked", "!": "Conflict" };

export function changeLabel(change: { code?: string; staged?: boolean }): string {
  const label = labels[change.code ?? ""] ?? "Changed";
  return change.staged ? `${label} (staged)` : label;
}

const byName = (a: ChangeNode, b: ChangeNode) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" });

// Builds the folder tree of changed files: folders first, then files, each sorted by name.
export function buildChangeTree(changes: GitChange[]): ChangeNode[] {
  const root: ChangeNode = { name: "", path: "", dir: true, children: [] };
  for (const change of changes) {
    const parts = change.path.split("/").filter(Boolean);
    let parent = root;
    parts.forEach((part, index) => {
      const path = parts.slice(0, index + 1).join("/");
      const last = index === parts.length - 1;
      let node = parent.children.find((child) => child.name === part && child.dir === !last);
      if (!node) {
        node = last ? { name: part, path, dir: false, code: change.code, staged: change.staged, children: [] } : { name: part, path, dir: true, children: [] };
        parent.children.push(node);
      }
      parent = node;
    });
  }
  const sort = (node: ChangeNode) => {
    node.children.sort((a, b) => (a.dir === b.dir ? byName(a, b) : a.dir ? -1 : 1));
    node.children.forEach(sort);
  };
  sort(root);
  return root.children;
}

export function countChanges(nodes: ChangeNode[]): number {
  return nodes.reduce((total, node) => total + (node.dir ? countChanges(node.children) : 1), 0);
}
