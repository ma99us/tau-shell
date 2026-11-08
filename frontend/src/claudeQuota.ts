export type ClaudeQuota = {
  host: string;
  spend: number;
  budget: number;
  budgetSource: "member" | "team" | "none";
  resetAt?: string;
  daysLeft: number;
  projected: number;
  canProject: boolean;
  teamSpend: number;
  teamBudget: number;
  teamName?: string;
};

export type ClaudeQuotaView = {
  budget: number;
  percent: number;
  overridden: boolean;
  overBudget: boolean;
};

// A positive manual budget replaces the one the proxy reports; the proxy value
// stays the default so clearing the override restores it.
export function claudeQuotaView(quota: ClaudeQuota, override: number | null): ClaudeQuotaView {
  const overridden = override !== null && override > 0;
  const budget = overridden ? override : quota.budget;
  return {
    budget,
    percent: budget > 0 ? (quota.spend / budget) * 100 : 0,
    overridden,
    overBudget: budget > 0 && (quota.spend > budget || (quota.canProject && quota.projected > budget)),
  };
}

// Accepts "2000", "$2,000" and "2000.50"; anything else clears the override.
export function parseBudgetInput(text: string): number | null {
  const value = Number(text.replace(/[$,\s]/g, ""));
  return Number.isFinite(value) && value > 0 ? Math.round(value * 100) / 100 : null;
}

export function formatDollars(value: number): string {
  return `$${value.toLocaleString("en-US", { maximumFractionDigits: 0 })}`;
}
