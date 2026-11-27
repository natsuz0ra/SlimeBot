import type { ContextUsage, UsageSummary, UsageTimeRange } from "../types.js";

export interface OutputBudgetStatus {
  used: number;
  budget: number;
  remaining: number;
  percent: number;
  tone: "normal" | "warning" | "danger";
  exceeded: boolean;
}

export function getOutputBudgetStatus(
  usage: Pick<ContextUsage, "outputBudgetTokens" | "outputBudgetUsed"> | null | undefined,
): OutputBudgetStatus | null {
  const budget = Number(usage?.outputBudgetTokens || 0);
  if (!Number.isFinite(budget) || budget <= 0) return null;
  const used = Math.max(0, Math.round(Number(usage?.outputBudgetUsed || 0)));
  const percent = Math.round((used / budget) * 100);
  const exceeded = used >= budget;
  const tone: OutputBudgetStatus["tone"] = exceeded ? "danger" : percent >= 80 ? "warning" : "normal";
  return {
    used,
    budget: Math.round(budget),
    remaining: Math.max(0, Math.round(budget) - used),
    percent,
    tone,
    exceeded,
  };
}

export function resolveUsageTimeRange(range: UsageTimeRange, now = new Date()): { since?: string; until?: string } {
  if (range === "all") return {};
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  if (range === "7d") start.setDate(start.getDate() - 6);
  else if (range === "30d") start.setDate(start.getDate() - 29);
  return { since: start.toISOString() };
}

export function formatCompactNumber(value: number): string {
  const n = Math.max(0, Math.round(value || 0));
  if (n < 1_000) return String(n);
  if (n < 1_000_000) return `${(n / 1_000).toFixed(1)}k`;
  return `${(n / 1_000_000).toFixed(2)}m`;
}

export function summarizeUsageRows(rows: UsageSummary[]): {
  requests: number;
  input: number;
  output: number;
  cacheRead: number;
} {
  return rows.reduce(
    (acc, row) => ({
      requests: acc.requests + (row.requestCount || 0),
      input: acc.input + (row.inputTokens || 0),
      output: acc.output + (row.outputTokens || 0),
      cacheRead: acc.cacheRead + (row.cacheReadInputTokens || 0),
    }),
    { requests: 0, input: 0, output: 0, cacheRead: 0 },
  );
}

export function formatUsageRowLabel(groupBy: "session" | "model" | "project", row: UsageSummary): string {
  if (groupBy === "session") {
    const name = row.sessionName?.trim();
    if (name) return row.sessionExists === false ? `${name} (deleted)` : name;
    return row.rootSessionId || row.groupKey || "(unknown session)";
  }
  if (groupBy === "model") {
    return row.model?.trim() || row.groupKey || "(unknown model)";
  }
  return row.workingDirectory?.trim() || row.groupKey || "(no workspace)";
}
