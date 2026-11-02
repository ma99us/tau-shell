import type { ControlAuditEntry } from "./bridge";

export const keepEntries = 50;

// Newest first, bounded so a chatty client cannot grow the list.
export function addAuditEntry(entries: ControlAuditEntry[], entry: ControlAuditEntry): ControlAuditEntry[] {
  return [entry, ...entries].slice(0, keepEntries);
}

export function auditCaller(entry: ControlAuditEntry): string {
  return `${entry.image || "unknown program"} (PID ${entry.pid})`;
}
