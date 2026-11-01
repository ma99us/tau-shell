export async function duplicateAndLaunch<T>(
  duplicate: () => Promise<T>,
  refresh: () => Promise<unknown>,
  launch: (session: T) => void | Promise<void>,
): Promise<T> {
  const copy = await duplicate();
  await refresh();
  await launch(copy);
  return copy;
}