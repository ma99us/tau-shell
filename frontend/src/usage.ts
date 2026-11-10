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
  firstStarted?: string;
  firstStartedApprox?: boolean;
  activeDays?: number;
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
  if (totals.usd <= 0) return totals.unpriced ? "$?" : "~$0";
  const whole = Math.ceil(totals.usd);
  return `~$${whole.toLocaleString("en-US")}${suffix}`;
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

export function formatAge(ms: number): string {
  const minutes = Math.max(0, Math.floor(ms / 60_000));
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes % 60}m`;
  return `${minutes}m`;
}

// Age and rough spend rates. Rates are averages over the whole age (idle time included) and per
// day with logged usage; priced usage only, so unpriced sessions show no rate.
export function ageLines(usage: SessionUsage, now: number): string[] {
  const started = usage.firstStarted ? Date.parse(usage.firstStarted) : NaN;
  if (Number.isNaN(started) || started > now) return [];
  const age = now - started;
  const lines = [`Age: ${usage.firstStartedApprox ? "~" : ""}${formatAge(age)} (first started ${new Date(started).toLocaleString()}${usage.firstStartedApprox ? ", from first logged usage day" : ""})`];
  if (usage.total.usd > 0) {
    const hours = age / 3_600_000;
    if (hours >= 1 / 6) lines.push(`Avg spend: ~$${(usage.total.usd / hours).toFixed(2)}/h over the session age`);
    if ((usage.activeDays ?? 0) > 0) lines.push(`~$${(usage.total.usd / usage.activeDays!).toFixed(2)} per day with usage (${usage.activeDays} day${usage.activeDays === 1 ? "" : "s"})`);
  }
  return lines;
}

export function usageTooltip(usage: SessionUsage, now: number = Date.now()): string {
  const lines = ["Estimate from harness logs; never reset", ...ageLines(usage, now)];
  for (const model of usage.models ?? []) lines.push(`${model.model}: ${formatTokens(model.tokens)} ${formatUSD(model)}`);
  if ((usage.conversations?.length ?? 0) > 1) lines.push(`${usage.conversations!.length} conversations`);
  lines.push(`Prices: ${usage.prices.source}${usage.prices.updatedAt ? ` (${usage.prices.updatedAt})` : ""}${usage.prices.stale ? " - stale" : ""}`);
  return lines.join("\n");
}
