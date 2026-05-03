export const CONTEXT_SIZE_MIN = 8_000;
export const CONTEXT_SIZE_MAX = 1_000_000;
export const CONTEXT_SIZE_DEFAULT = 1_000_000;

export function clampContextSize(value: number | string | null | undefined): number {
  const numeric = typeof value === "string" ? Number(value.replace(/,/g, "").trim()) : Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return CONTEXT_SIZE_DEFAULT;
  return Math.min(CONTEXT_SIZE_MAX, Math.max(CONTEXT_SIZE_MIN, Math.round(numeric)));
}

export function formatContextSize(value: number): string {
  const clamped = clampContextSize(value);
  if (clamped >= 1_000_000) return "1M";
  if (clamped % 1_000 === 0) return `${clamped / 1_000}K`;
  return clamped.toLocaleString("en-US");
}

export function formatContextTokenCount(tokens: number): string {
  const count = Math.max(0, Math.round(tokens));
  if (count < 1_000) return `${count} tokens`;
  if (count < 1_000_000) return `${(count / 1_000).toFixed(1)}k tokens`;
  return `${(count / 1_000_000).toFixed(1)}m tokens`;
}

export type ContextUsageEstimate = {
  usedTokens: number;
  totalTokens: number;
  usedPercent: number;
  availablePercent: number;
};

export function estimateContextTextTokens(text: string): number {
  const runes = Array.from(text || "").length;
  if (runes === 0) return 0;
  return Math.ceil(runes / 4);
}

export function estimateContextUsageWithText<T extends ContextUsageEstimate | null | undefined>(
  usage: T,
  text: string,
): T {
  if (!usage) return usage;
  const delta = estimateContextTextTokens(text);
  if (delta <= 0) return usage;
  const total = Math.max(0, Math.round(usage.totalTokens));
  const nextUsed = total > 0
    ? Math.min(total, Math.max(0, Math.round(usage.usedTokens) + delta))
    : Math.max(0, Math.round(usage.usedTokens) + delta);
  const usedPercent = total > 0
    ? Math.max(0, Math.min(100, Math.round((nextUsed / total) * 100)))
    : 0;
  return {
    ...usage,
    usedTokens: nextUsed,
    usedPercent,
    availablePercent: 100 - usedPercent,
  };
}

export function formatContextUsageStatus(
  usage: {
    usedTokens: number;
    totalTokens: number;
    usedPercent: number;
    isCompacted?: boolean;
  } | null | undefined,
  width: number,
): string {
  if (!usage) return "";
  const base = `CTX ${Math.max(0, Math.min(100, Math.round(usage.usedPercent)))}%`;
  if (width < 24) return base;
  const full = `${base} · ${formatContextTokenCount(usage.usedTokens)}/${formatContextTokenCount(usage.totalTokens)}${usage.isCompacted ? " · compacted" : ""}`;
  if (width < full.length + 4) return base;
  return full;
}

export function adjustContextSize(value: number, delta: number): number {
  return clampContextSize(value + delta);
}

export function renderContextSizeBar(value: number, width: number): string {
  const clamped = clampContextSize(value);
  const columns = Math.max(8, width);
  const ratio = (clamped - CONTEXT_SIZE_MIN) / (CONTEXT_SIZE_MAX - CONTEXT_SIZE_MIN);
  const filled = Math.max(0, Math.min(columns, Math.round(ratio * columns)));
  return `${"=".repeat(filled)}${"-".repeat(columns - filled)}`;
}
