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
