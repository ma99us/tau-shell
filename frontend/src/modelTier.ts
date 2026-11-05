// Models whose per-token price is at the top of a vendor's range (Opus, "pro",
// "max" and reasoning-only tiers). A name heuristic: the harness reports only
// the model id, so new expensive models may need adding here.
export function isExpensiveModel(model: string): boolean {
  const id = model.toLowerCase();
  if (/\b(mini|nano|flash|haiku|sonnet|lite)\b|-(mini|nano|flash|haiku|sonnet|lite)\b/.test(id)) return false;
  return /opus|ultra|(^|[^a-z0-9])(pro|max)($|[^a-z0-9])|(^|[^a-z0-9])o[13]($|[^a-z0-9])/.test(id);
}
