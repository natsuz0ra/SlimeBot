import wrapAnsi from "wrap-ansi";

/** Formats tool invocation text shown in timeline rows. */
export function formatToolInvocation(
  toolName: string,
  command: string,
  params?: Record<string, string>,
): string {
  const name = toolName.trim() || "tool";
  const cmd = command.trim() || "run";

  if (!params || Object.keys(params).length === 0) {
    return `${name}.${cmd}()`;
  }

  const keys = Object.keys(params).sort();
  const parts = keys.map((k) => `${k}=${formatParamValue(params[k])}`);
  return `${name}.${cmd}(${parts.join(", ")})`;
}

function formatParamValue(value: string): string {
  const v = value.trim();
  if (!v) return '""';
  if (/[\s\t\r\n,()]/.test(v)) return JSON.stringify(v);
  return v;
}

/** Truncates multi-line text into a single-line preview. */
export function truncateText(text: string, maxLen: number): string {
  const singleLine = text.replace(/\r?\n/g, " ").replace(/\s+/g, " ").trim();
  if (!singleLine) return "(No output)";
  if (singleLine.length <= maxLen) return singleLine;
  const suffix = "… [truncated]";
  return singleLine.slice(0, maxLen - suffix.length) + suffix;
}

/** Default number of preview lines shown when tool output is collapsed. */
export const TOOL_OUTPUT_PREVIEW_LINES = 5;

/**
 * Formats tool output lines with collapsible support.
 * Returns the lines to display and the total line count.
 * - Short output (≤ maxPreviewLines): all lines, no hint.
 * - Collapsed: first maxPreviewLines lines + expand hint.
 * - Expanded: all lines + collapse hint.
 */
export function formatCollapsedLines(
  text: string,
  maxPreviewLines: number,
  expanded: boolean,
): { lines: string[]; totalLines: number } {
  const normalized = (text ?? "").replace(/\r\n/g, "\n").trim();
  if (!normalized) {
    return { lines: ["(No output)"], totalLines: 1 };
  }

  const rawLines = normalized.split("\n");
  const totalLines = rawLines.length;

  if (totalLines <= maxPreviewLines) {
    return { lines: rawLines, totalLines };
  }

  if (expanded) {
    return {
      lines: [...rawLines, "… (ctrl+o to collapse)"],
      totalLines,
    };
  }

  const preview = rawLines.slice(0, maxPreviewLines);
  const remaining = totalLines - maxPreviewLines;
  preview.push(`… +${remaining} more lines (ctrl+o to expand)`);
  return { lines: preview, totalLines };
}

/** Pre-wraps text for terminal width, preserving ANSI and CJK width. */
export function wrapText(text: string, maxWidth: number): string {
  const normalized = (text ?? "").replace(/\r\n/g, "\n");
  const width = Math.max(1, Math.floor(maxWidth));
  return wrapAnsi(normalized, width, {
    hard: true,
    trim: false,
  });
}

/** Format ISO timestamp into local readable date-time string. */
export function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}
