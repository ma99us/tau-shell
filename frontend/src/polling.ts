export function shouldReportPollFailure(failures: Set<string>, key: string): boolean {
  if (failures.has(key)) return false;
  failures.add(key);
  return true;
}

export function clearPollFailure(failures: Set<string>, key: string): void {
  failures.delete(key);
}