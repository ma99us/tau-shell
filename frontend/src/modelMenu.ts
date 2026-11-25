import type { ModelMenu, ModelOption } from "./bridge";

export const topModelCount = 8;
const otherModelCap = 30;

export type ModelSections = { top: ModelOption[]; other: ModelOption[]; hidden: number };

export function splitModels(options: ModelOption[], topCount = topModelCount): ModelSections {
  const ranked = options.filter((option) => option.ranked);
  const top = ranked.slice(0, topCount);
  const rest = [...ranked.slice(topCount), ...options.filter((option) => !option.ranked)];
  return { top, other: rest.slice(0, otherModelCap), hidden: Math.max(0, rest.length - otherModelCap) };
}

export function scoreBadge(option: ModelOption): string {
  const rough = option.confidence && !/confident/i.test(option.confidence) ? "~" : "";
  return `${rough}${option.score.toFixed(1)}`;
}

export function costBadge(option: ModelOption): string {
  return option.cost > 0 ? `${option.cost.toFixed(option.cost < 10 ? 1 : 0)}x` : "";
}

// A quiet one-line note on where the ranking came from; never an error dialog.
export function rankingStatus(menu: ModelMenu): string {
  const { source, asOf, stale, error } = menu.ranking;
  if (source === "none") return "Ranking unavailable (MEI offline). Showing models unranked.";
  const origin = source === "snapshot" ? "built-in snapshot" : `MEI data as of ${asOf}`;
  const notes = [stale ? "stale" : "", error ? "refresh failed" : ""].filter(Boolean);
  return notes.length ? `${origin}, ${notes.join(", ")}` : origin;
}

export function fallbackMenu(models: string[]): ModelMenu {
  return {
    options: models.map((id) => ({ id, label: id, ranked: false, score: 0, cost: 0, tokens: 0 })),
    ranking: { source: "none", stale: false },
    probing: false,
  };
}
