// Presentation state persisted in the WebView's localStorage. The WebView profile
// can be missing, cleared, full, or hold hand-edited/older data, so every read
// falls back to a default and every failure is reported instead of breaking the UI.
export type Tab = { id: string; name: string };
export const savedTabsKey = "taushell.tabs.v1";
export const recentFoldersKey = "taushell.recent-folders.v1";
export const maxRecentFolders = 5;
export const groupedKey = "taushell.home-grouped.v1";

// Home groups sessions by workspace unless the user switched that off.
export function readGrouped(storage: Pick<Storage, "getItem">): boolean {
  try {
    return storage.getItem(groupedKey) !== "false";
  } catch {
    return true;
  }
}

type Report = (source: string, error: unknown) => void;

function readJsonArray(key: string, source: string, report: Report): unknown[] {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(key) ?? "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch (error) {
    report(source, error);
    return [];
  }
}

export function readRecentFolders(report: Report): string[] {
  return readJsonArray(recentFoldersKey, "storage.recent-folders", report)
    .filter((folder): folder is string => typeof folder === "string" && folder.length > 0)
    .slice(0, maxRecentFolders);
}

export function readTabs(report: Report): Tab[] {
  const tabs = readJsonArray(savedTabsKey, "storage.tabs", report)
    .filter((tab): tab is Tab => typeof (tab as Tab | null)?.id === "string" && typeof (tab as Tab).name === "string")
    .map((tab) => ({ id: tab.id, name: tab.name }));
  return tabs.filter((tab, index) => tabs.findIndex((other) => other.id === tab.id) === index);
}

export function writeStored(key: string, value: unknown, report: Report): void {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch (error) {
    report(`storage.write.${key}`, error);
  }
}
