import type { ReactNode } from "react";

// Simple stroke icons (24x24 grid). TauShell-owned, no external assets.
const paths: Record<string, ReactNode> = {
  files: <><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5v9a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5Z" /></>,
  prompts: <><path d="M4 5h16v11H9l-5 4Z" /><path d="M8 9.5h8M8 12.5h5" /></>,
  queue: <><path d="M4 6h12M4 12h12M4 18h7" /><path d="m17 15 4 3-4 3Z" /></>,
  commands: <><rect x="3.5" y="3.5" width="17" height="17" rx="3" /><path d="M13.5 8 10.5 16" /></>,
  model: <><rect x="6" y="6" width="12" height="12" rx="2" /><path d="M9 3v3M15 3v3M9 18v3M15 18v3M3 9h3M3 15h3M18 9h3M18 15h3" /></>,
  compact: <><path d="M19.5 4.5 12.5 11.5" /><path d="M10 9l5 5-5.2 5.2H4.5V14Z" /><path d="M7.5 16.5l1.8-1.8M10 19l2-2" /></>,
  clear: <><path d="M4 7h16" /><path d="M9 7V4h6v3" /><path d="M6.5 7 7.5 20h9l1-13" /><path d="M10 11v5M14 11v5" /></>,
  interrupt: <><rect x="6" y="6" width="12" height="12" rx="2" /></>,
  folder: <><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5v9a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 17.5Z" /></>,
  folderOpen: <><path d="M3 18V6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h7.5A1.5 1.5 0 0 1 20 8.5V10" /><path d="M3 18l2.5-7h16.2l-2.4 7Z" /></>,
  doc: <><path d="M6 3h8l4 4v14H6Z" /><path d="M14 3v4h4" /><path d="M9 12h6M9 15.5h6" /></>,
  markdown: <><rect x="3" y="6" width="18" height="12" rx="2" /><path d="M6.5 15V9l2.5 3 2.5-3v6M16 9v6m-2-2 2 2 2-2" /></>,
  code: <><path d="m9 7-5 5 5 5" /><path d="m15 7 5 5-5 5" /></>,
  data: <><path d="M8 4C6 4 6 5 6 7v2c0 1.5-1 2-2 3 1 1 2 1.5 2 3v2c0 2 0 3 2 3" /><path d="M16 4c2 0 2 1 2 3v2c0 1.5 1 2 2 3-1 1-2 1.5-2 3v2c0 2 0 3-2 3" /></>,
  image: <><rect x="3" y="5" width="18" height="14" rx="2" /><circle cx="9" cy="10" r="1.5" /><path d="m4 18 5-5 4 4 3-3 4 4" /></>,
  shell: <><rect x="3" y="5" width="18" height="14" rx="2" /><path d="m7 10 3 2-3 2M12 15h5" /></>,
  archive: <><rect x="4" y="4" width="16" height="16" rx="2" /><path d="M12 4v10M10 8h2M12 11h2" /></>,
  gear: <><circle cx="12" cy="12" r="3" /><path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1" /></>,
  lock: <><rect x="5" y="11" width="14" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></>,
  copy: <><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3" /></>,
  sessionFile: <><path d="M6 3h8l4 4v14H6Z" /><path d="M14 3v4h4" /><path d="m10 12-2 2 2 2M14 12l2 2-2 2" /></>,
  groupBy: <><rect x="3" y="4" width="18" height="5" rx="1.5" /><rect x="3" y="13" width="8" height="7" rx="1.5" /><rect x="13" y="13" width="8" height="7" rx="1.5" /></>,
  sortRecent: <><path d="M6 4v15M2.5 15.5 6 19l3.5-3.5" /><circle cx="16.5" cy="11" r="5" /><path d="M16.5 8.5V11l1.8 1.2" /></>,
  edit: <><path d="M4 20h4L19 9l-4-4L4 16Z" /><path d="m13.5 6.5 4 4" /></>,
  external: <><path d="M14 4h6v6" /><path d="M20 4l-9 9" /><path d="M18 14v4.5a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 4 18.5v-11A1.5 1.5 0 0 1 5.5 6H10" /></>,
  reveal: <><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H9l2 2h8.5A1.5 1.5 0 0 1 21 8.5V10" /><path d="M3 18V8M3 18h10" /><path d="M15 21l6-6m0 0h-4.5M21 15v4.5" /></>,
  history: <><path d="M3.5 12a8.5 8.5 0 1 0 2.6-6.1L3.5 8.5" /><path d="M3.5 4v4.5H8" /><path d="M12 7.5V12l3 2" /></>,
  sync: <><path d="M20 11a8 8 0 0 0-14-4.5L4 9" /><path d="M4 4v5h5" /><path d="M4 13a8 8 0 0 0 14 4.5l2-2.5" /><path d="M20 20v-5h-5" /></>,
  chevron: <path d="M9 6l6 6-6 6" />,
  arrowUp: <path d="M12 19V5M6 11l6-6 6 6" />,
  arrowDown: <path d="M12 5v14M6 13l6 6 6-6" />,
  play: <path d="M7 5l12 7-12 7z" />,
  stop: <rect x="6" y="6" width="12" height="12" rx="1.5" />,
  branch: <><path d="M6 3v12" /><circle cx="18" cy="6" r="3" /><circle cx="6" cy="18" r="3" /><path d="M18 9a9 9 0 0 1-9 9" /></>,
  worktree: <><path d="M9 20H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.81 1.2a2 2 0 0 0 1.67.9H20a2 2 0 0 1 2 2v5" /><circle cx="13" cy="12" r="2" /><path d="M18 19c-2.8 0-5-2.2-5-5v8" /><circle cx="20" cy="19" r="2" /></>,
  pullRequest: <><circle cx="6" cy="5.5" r="2" /><circle cx="6" cy="18.5" r="2" /><circle cx="18" cy="18.5" r="2" /><path d="M6 7.5v9M18 16.5V9a2 2 0 0 0-2-2h-3m2-2.5L13 7l2 2.5" /></>,
};

export type IconName = keyof typeof paths;

export function Icon({ name, size = 16 }: { name: IconName; size?: number }) {
  return <svg className={`ui-icon ui-icon-${name}`} viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">{paths[name]}</svg>;
}
