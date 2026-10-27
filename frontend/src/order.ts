// Returns ids with `fromId` moved directly before or after `toId`.
export function moveId(ids: string[], fromId: string, toId: string, after: boolean): string[] {
  if (fromId === toId || !ids.includes(fromId) || !ids.includes(toId)) return ids;
  const rest = ids.filter((id) => id !== fromId);
  const index = rest.indexOf(toId) + (after ? 1 : 0);
  return [...rest.slice(0, index), fromId, ...rest.slice(index)];
}
