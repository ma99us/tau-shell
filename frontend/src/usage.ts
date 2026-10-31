export type UsageTotals = {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  tokens: number;
  usd: number;
  unpriced: boolean;
};

export type PriceInfo = { source: string; updatedAt: string; stale: boolean; overrides: boolean };

export type SessionUsage = {
  total: UsageTotals;
  models: (UsageTotals & { model: string })[] | null;
  conversations: (UsageTotals & { harnessSessionId: string; firstDay: string; lastDay: string })[] | null;
  prices: PriceInfo;
};

export type UsageSummary = {
  sessions: Record<string, UsageTotals> | null;
  month: UsageTotals;
  monthByTool: Record<string, UsageTotals> | null;
  prevMonth: UsageTotals;
  prevMonthByTool: Record<string, UsageTotals> | null;
  monthStart: string;
  prevMonthStart: string;
  prices: PriceInfo;
};

export function formatTokens(count: number): string {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(count >= 10_000_000 ? 0 : 1)}M tok`;
  if (count >= 1_000) return `${(count / 1_000).toFixed(count >= 100_000 ? 0 : 1)}k tok`;
  return `${count} tok`;
}

// A trailing "+" means some usage had no known price and is not in the amount.
export function formatUSD(totals: UsageTotals): string {
  const suffix = totals.unpriced ? "+" : "";
  if (totals.usd <= 0) return totals.unpriced ? "$?" : "~$0.00";
  return `~$${totals.usd < 10 ? totals.usd.toFixed(2) : totals.usd.toFixed(totals.usd < 100 ? 1 : 0)}${suffix}`;
}

export function hasUsage(totals?: UsageTotals | null): totals is UsageTotals {
  return !!totals && (totals.tokens > 0 || totals.usd > 0);
}

export function usageText(totals: UsageTotals): string {
  return `${formatTokens(totals.tokens)}, ${formatUSD(totals)}`;
}

export function monthTooltip(summary: UsageSummary): string {
  const lines = [`Since ${summary.monthStart}; includes deleted sessions`];
  if (hasUsage(summary.prevMonth)) lines.push(`Last month (from ${summary.prevMonthStart}): ${usageText(summary.prevMonth)}`);
  lines.push(`Claude prices: ${summary.prices.source}${summary.prices.stale ? " - stale" : ""}`);
  return lines.join("\n");
}

export function usageTooltip(usage: SessionUsage): string {
  const lines = ["Estimate from harness logs; never reset"];
  for (const model of usage.models ?? []) lines.push(`${model.model}: ${formatTokens(model.tokens)} ${formatUSD(model)}`);
  if ((usage.conversations?.length ?? 0) > 1) lines.push(`${usage.conversations!.length} conversations`);
  lines.push(`Prices: ${usage.prices.source}${usage.prices.updatedAt ? ` (${usage.prices.updatedAt})` : ""}${usage.prices.stale ? " - stale" : ""}`);
  return lines.join("\n");
}
