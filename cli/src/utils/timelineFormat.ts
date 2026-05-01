import { renderMarkdownLines } from "./markdownRenderer.js";
import { DOT } from "./terminal.js";
import { stringWidth } from "./stringWidth.js";
import type { RuntimeTodoItem, TimelineEntry, ToolCallStatus } from "../types.js";
import {
  formatToolInvocation,
  formatToolCallSummary,
  wrapText,
  formatCollapsedLines,
  TOOL_OUTPUT_PREVIEW_LINES,
  formatToolExecutionOutput,
  formatToolExecutionCompactOutput,
  formatToolTextValue,
  formatToolParamEntries,
  filterToolParamsForDetail,
  truncateText,
} from "./format.js";
import {
  buildFileToolDisplays,
  type FileDiffLine,
  isFileToolEntry as isFileToolTimelineEntry,
} from "./fileToolDisplay.js";

export const PLAN_GOLD = "#f59e0b";
export const WAITING_STATS_COLOR = "#64748b";
export const TOOL_SUMMARY_TAG_COLOR = "#7dd3fc";
export const SHOW_CLI_THINKING = false;

export function toolDotState(status: ToolCallStatus): { color: string; blink: boolean } {
  switch (status) {
    case "pending":
    case "executing":
      return { color: "#B8860B", blink: true };
    case "completed":
      return { color: "#2E7D32", blink: false };
    case "error":
    case "rejected":
      return { color: "#C62828", blink: false };
    default:
      return { color: "white", blink: false };
  }
}

export function formatToolStatusPart(status: ToolCallStatus): { text: string; color: string } {
  switch (status) {
    case "pending":
      return { text: "? pending approval", color: "#B8860B" };
    case "executing":
      return { text: "… executing", color: "#B8860B" };
    case "completed":
      return { text: "✓ completed", color: "#2E7D32" };
    case "error":
      return { text: "✕ failed", color: "#C62828" };
    case "rejected":
      return { text: "✕ rejected", color: "#C62828" };
    default:
      return { text: "", color: "white" };
  }
}

export function formatToolSummaryTag(summary: string): string {
  const trimmed = summary.trim();
  return trimmed ? `[${trimmed}]` : "";
}

export function formatSubagentStreamLines(stream: string, maxWidth: number, expanded: boolean): string[] {
  const { lines } = formatCollapsedLines(stream.trim(), TOOL_OUTPUT_PREVIEW_LINES, expanded);
  const linePrefix = "   |> ";
  const contentWidth = Math.max(1, maxWidth - linePrefix.length);
  const out: string[] = [];
  for (const line of lines) {
    const wrapped = wrapText(line, contentWidth);
    for (const sub of wrapped.split("\n")) {
      out.push(`${linePrefix}${sub}`);
    }
  }
  return out;
}

export function formatToolOutputLines(entry: TimelineEntry, maxWidth: number, expanded: boolean): string[] {
  const outputPrefix = "   => ";
  const continuationPrefix = "      ";
  const prefixWidth = outputPrefix.length;
  const contentWidth = Math.max(1, maxWidth - prefixWidth);
  const raw = (entry.status === "error" || entry.status === "rejected")
    ? (entry.error || entry.content)
    : (entry.output || entry.content);
  const normalizedTool = (entry.toolName || "").trim().toLowerCase();
  const normalizedCommand = (entry.command || "").trim().toLowerCase();
  const useExecCompact = !expanded && normalizedTool === "exec" && normalizedCommand === "run";
  const formatted = useExecCompact
    ? formatToolExecutionCompactOutput(entry.toolName || "", entry.command || "", raw || "")
    : formatToolExecutionOutput(entry.toolName || "", entry.command || "", raw || "");
  const { lines: rawLines } = useExecCompact
    ? { lines: formatted.split("\n"), totalLines: formatted.split("\n").length }
    : formatCollapsedLines(formatted, TOOL_OUTPUT_PREVIEW_LINES, expanded);
  const result: string[] = [];
  for (const line of rawLines) {
    const wrapped = wrapText(line, contentWidth);
    const subLines = wrapped.split("\n");
    for (const sub of subLines) {
      result.push(result.length === 0 ? `${outputPrefix}${sub}` : `${continuationPrefix}${sub}`);
    }
  }
  return result;
}

export function formatToolParamLines(entry: TimelineEntry, maxWidth: number, expanded = false): string[] {
  const params = filterToolParamsForDetail(entry.toolName || "", entry.command || "", entry.params);
  const normalizedTool = (entry.toolName || "").trim().toLowerCase();
  const normalizedCommand = (entry.command || "").trim().toLowerCase();
  if (normalizedTool === "exec" && normalizedCommand === "run") {
    if (!expanded) {
      delete params.command;
    } else if (entry.params && entry.params.command !== undefined) {
      params.command = entry.params.command;
    }
  }
  return formatToolParamLinesForParams(params, maxWidth);
}

export function isFileToolEntry(entry: TimelineEntry): boolean {
  return isFileToolTimelineEntry(entry);
}

function fileDiffLineText(line: FileDiffLine): string {
  const marker = line.kind === "added" ? "+" : line.kind === "removed" ? "-" : " ";
  const lineNo = line.kind === "added" ? line.newLine : line.oldLine ?? line.newLine;
  const paddedLineNo = lineNo === undefined ? "   " : String(lineNo).padStart(3, " ");
  return `${marker} ${paddedLineNo}  ${line.text}`;
}

function treeWrapLine(prefix: string, text: string, maxWidth: number): string[] {
  const contentWidth = Math.max(1, maxWidth - prefix.length);
  const wrapped = wrapText(text, contentWidth).split("\n");
  return wrapped.map((line, index) => `${index === 0 ? prefix : " ".repeat(prefix.length)}${line}`);
}

export function formatFileToolTimelineLines(entry: TimelineEntry, maxWidth: number, expanded: boolean): string[] {
  const displays = buildFileToolDisplays(entry);
  if (displays.length === 0) return [];

  if (entry.status === "error" || entry.status === "rejected") {
    const message = entry.error || entry.content || "File tool failed";
    return treeWrapLine("   └─ ", message, maxWidth);
  }

  if (entry.status !== "completed") {
    return [];
  }

  const lines: string[] = [];
  for (const display of displays) {
    lines.push(...treeWrapLine("   └─ ", display.summary, maxWidth));
    if (display.toolName === "file_read" || display.diffLines.length === 0) {
      continue;
    }

    const maxPreviewLines = 8;
    const isFileEdit = display.toolName === "file_edit";
    const diffLines = isFileEdit ? display.diffLines : (expanded ? display.diffLines : display.diffLines.slice(0, maxPreviewLines));
    const remaining = display.diffLines.length - diffLines.length;
    const rows = diffLines.map(fileDiffLineText);
    const separatorLine = "─".repeat(Math.max(8, maxWidth - 12));
    if (remaining > 0) {
      rows.push(isFileEdit ? `+${remaining} more changed lines` : `... +${remaining} more changed lines (ctrl+o to expand)`);
    } else if (expanded && !isFileEdit && display.diffLines.length > maxPreviewLines) {
      rows.push("... (ctrl+o to collapse)");
    }

    rows.forEach((row, index) => {
      const connector = index === rows.length - 1 ? "└─ " : "├─ ";
      if (index > 0 && row.startsWith("+") === false && diffLines[index] && diffLines[index - 1]) {
        const prev = diffLines[index - 1]!;
        const curr = diffLines[index]!;
        const prevNo = prev.newLine ?? prev.oldLine;
        const currNo = curr.newLine ?? curr.oldLine;
        if (prevNo !== undefined && currNo !== undefined && currNo - prevNo > 1) {
          lines.push(...treeWrapLine("      ├─ ", separatorLine, maxWidth));
        }
      }
      lines.push(...treeWrapLine(`      ${connector}`, row, maxWidth));
    });
  }
  return lines;
}

function formatToolParamLinesForParams(params: Record<string, unknown> | undefined, maxWidth: number): string[] {
  const paramPrefix = "   :: ";
  const continuationPrefix = "      ";
  const prefixWidth = paramPrefix.length;
  const contentWidth = Math.max(1, maxWidth - prefixWidth);
  const rawLines = formatToolParamEntries(params);
  const result: string[] = [];
  for (const line of rawLines) {
    const wrapped = wrapText(line, contentWidth);
    const subLines = wrapped.split("\n");
    for (const sub of subLines) {
      result.push(result.length === 0 ? `${paramPrefix}${sub}` : `${continuationPrefix}${sub}`);
    }
  }
  return result;
}

export function formatThinkingLabel(entry: TimelineEntry): string {
  const done = entry.thinkingDone;
  const duration = done && entry.thinkingDurationMs !== undefined
    ? (entry.thinkingDurationMs / 1000).toFixed(1) + "s"
    : done && entry.thinkingStartedAt
    ? ((Date.now() - entry.thinkingStartedAt) / 1000).toFixed(1) + "s"
    : "";
  return done ? `Thought for ${duration}` : "Thinking...";
}

export function formatSubagentThinkingLines(entry: TimelineEntry, maxWidth: number, expanded = false): string[] {
  if (!entry.subagentThinking) return [];
  const thinking = entry.subagentThinking;
  const done = thinking.thinkingDone;
  const duration = done && thinking.thinkingDurationMs !== undefined
    ? (thinking.thinkingDurationMs / 1000).toFixed(1) + "s"
    : "";
  const label = done ? `Sub-agent thought for ${duration}` : "Sub-agent thinking...";
  const lines = [`   ${label}`];
  if (thinking.content.trim() !== "") {
    const { lines: rawLines } = formatCollapsedLines(thinking.content, TOOL_OUTPUT_PREVIEW_LINES, expanded);
    for (const raw of rawLines) {
      const wrapped = wrapText(raw, Math.max(20, maxWidth - 6));
      lines.push(...wrapped.split("\n").map((line) => `     ${line}`));
    }
  }
  return lines;
}

export function isRunSubagentEntry(entry: TimelineEntry): boolean {
  return (entry.toolName || "").trim().toLowerCase() === "run_subagent";
}

function displayParamValue(params: Record<string, unknown> | undefined, key: string): string {
  return formatToolTextValue(String(params?.[key] ?? "")).trim();
}

function summarizeMultilineText(text: string, maxLen: number): string {
  const normalized = text.trim().replace(/\r\n/g, "\n");
  if (!normalized) return "(No output)";
  const rawLines = normalized.split("\n");
  const firstLine = truncateText(rawLines[0] || "", maxLen);
  if (rawLines.length <= 1) return firstLine;
  return `${firstLine} ... +${rawLines.length - 1} more lines`;
}

function runSubagentExtraParams(params: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  if (!params) return undefined;
  const filtered: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(params)) {
    const normalized = key.trim().toLowerCase();
    if (normalized === "context" || normalized === "task" || normalized === "title") continue;
    filtered[key] = value;
  }
  return Object.keys(filtered).length > 0 ? filtered : undefined;
}

function formatActiveSubagentToolSummary(child: TimelineEntry): string {
  const toolName = (child.toolName || "tool").trim() || "tool";
  const description = formatToolCallSummary(child.toolName || "", child.command || "", child.params).trim()
    || formatToolInvocation(child.toolName || "", child.command || "");
  return description ? `${toolName} [${description}]` : toolName;
}

function formatRunSubagentThinkingToolsSummary(entry: TimelineEntry, nestedTools: TimelineEntry[]): string {
  if (entry.status === "pending" || entry.status === "executing") {
    if (entry.subagentThinking && !entry.subagentThinking.thinkingDone) {
      return "Thinking...";
    }

    const activeTool = [...nestedTools].reverse().find((child) =>
      child.status === "executing" || child.status === "pending"
    );
    if (activeTool) {
      return formatActiveSubagentToolSummary(activeTool);
    }

    return nestedTools.length > 0
      ? `${nestedTools.length} tool${nestedTools.length === 1 ? "" : "s"}`
      : "working...";
  }

  return nestedTools.length > 0
    ? `${nestedTools.length} tool${nestedTools.length === 1 ? "" : "s"}`
    : "No tools";
}

type RunSubagentTreeItem = {
  label: string;
  summary: string;
  detailLines: string[];
};

function treeConnector(index: number, total: number): string {
  return index === total - 1 ? "└─" : "├─";
}

function treeDetailPrefix(index: number, total: number): string {
  return index === total - 1 ? "   " : "│  ";
}

function formatRunSubagentTreeLines(lines: string[], maxWidth: number): string[] {
  const safeWidth = Math.max(1, Math.floor(maxWidth) - 2);
  const items: RunSubagentTreeItem[] = [];
  const sectionPattern = /^(Context|Task|Thinking & tools|Params|Result):\s*(.*)$/;

  for (const line of lines) {
    const trimmed = line.trim();
    const match = trimmed.match(sectionPattern);
    if (match) {
      items.push({ label: match[1]!, summary: match[2] || "", detailLines: [] });
      continue;
    }
    const current = items[items.length - 1];
    if (current && trimmed) {
      current.detailLines.push(trimmed);
    }
  }

  const result: string[] = [];
  for (let i = 0; i < items.length; i++) {
    const item = items[i]!;
    const separator = item.label === "Thinking & tools" ? ": " : " → ";
    const summaryPrefix = `   ${treeConnector(i, items.length)} ${item.label}${item.summary ? separator : ""}`;
    if (item.summary) {
      const wrapped = wrapText(item.summary, Math.max(1, safeWidth - summaryPrefix.length)).split("\n");
      result.push(`${summaryPrefix}${wrapped[0] ?? ""}`);
      const continuationPrefix = `   ${treeDetailPrefix(i, items.length)}`.padEnd(summaryPrefix.length, " ");
      for (const part of wrapped.slice(1)) {
        result.push(`${continuationPrefix}${part}`);
      }
    } else {
      result.push(...wrapText(summaryPrefix, safeWidth).split("\n"));
    }

    const detailPrefix = `   ${treeDetailPrefix(i, items.length)}  `;
    for (const detail of item.detailLines) {
      const wrapped = wrapText(detail, Math.max(1, safeWidth - detailPrefix.length));
      result.push(...wrapped.split("\n").map((part) => `${detailPrefix}${part}`));
    }
  }
  return result;
}

export function getRunSubagentDetailLineColor(line: string, previousColor: string = "gray"): string {
  if (line.includes("Thinking & tools")) return "cyan";
  if (line.includes("Context") || line.includes("Task") || line.includes("Result")) return "white";
  if (line.includes("Params")) return "gray";
  return previousColor === "white" ? "white" : "gray";
}

function formatSubagentSectionLines(label: string, body: string, maxWidth: number, expanded: boolean): string[] {
  const normalized = body.trim();
  if (!normalized) return [];
  const prefix = `   ${label}: `;
  const detailPrefix = "      ";
  const contentWidth = Math.max(1, maxWidth - prefix.length);
  if (!expanded) {
    return [`${prefix}${summarizeMultilineText(normalized, contentWidth)}`];
  }

  const { lines } = formatCollapsedLines(normalized, TOOL_OUTPUT_PREVIEW_LINES, true);
  const result = [`   ${label}:`];
  for (const line of lines) {
    const wrapped = wrapText(line, Math.max(1, maxWidth - detailPrefix.length));
    result.push(...wrapped.split("\n").map((sub) => `${detailPrefix}${sub}`));
  }
  return result;
}

export function formatRunSubagentDetailLines(
  entry: TimelineEntry,
  nestedTools: TimelineEntry[] = [],
  maxWidth: number,
  expanded: boolean,
): string[] {
  const context = displayParamValue(entry.params, "context");
  const task = displayParamValue(entry.params, "task") || (entry.subagentTask || "");
  const lines: string[] = [];

  lines.push(...formatSubagentSectionLines("Context", context, maxWidth, expanded));
  lines.push(...formatSubagentSectionLines("Task", task, maxWidth, expanded));

  const thinkingToolsSummary = formatRunSubagentThinkingToolsSummary(entry, nestedTools);
  lines.push(`   Thinking & tools: ${thinkingToolsSummary}${expanded ? " (ctrl+o to collapse)" : " (ctrl+o to expand)"}`);

  if (expanded) {
    for (const child of nestedTools) {
      lines.push(...formatToolParamLinesForParams({ tool: formatToolInvocation(child.toolName || "", child.command || "") }, maxWidth));
      if (child.status === "completed" || child.status === "error" || child.status === "rejected") {
        lines.push(...formatToolOutputLines(child, maxWidth, true));
      }
    }
  }

  const extraParamLines = formatToolParamLinesForParams(runSubagentExtraParams(entry.params), maxWidth);
  if (extraParamLines.length > 0) {
    lines.push("   Params:");
    lines.push(...extraParamLines);
  }

  const resultRaw = (entry.status === "error" || entry.status === "rejected")
    ? (entry.error || entry.content || "")
    : (entry.output || entry.subagentStream || entry.content || "");
  if (resultRaw.trim()) {
    if (expanded) {
      lines.push("   Result:");
      lines.push(...formatToolOutputLines({ ...entry, output: resultRaw }, maxWidth, true));
    } else {
      const formatted = formatToolExecutionOutput(entry.toolName || "", entry.command || "", resultRaw);
      lines.push(`   Result: ${summarizeMultilineText(formatted, Math.max(20, maxWidth - 14))} (ctrl+o to expand)`);
    }
  } else if (entry.status === "executing" || entry.status === "pending") {
    lines.push("   Result: waiting for sub-agent output");
  }

  return formatRunSubagentTreeLines(lines, maxWidth);
}

export function formatPlanningIndicatorParts(blinkOn: boolean): { dot: string; label: string; color: string } {
  return {
    dot: blinkOn ? DOT : " ",
    label: "Planning...",
    color: PLAN_GOLD,
  };
}

export function shouldShowWaitingPrompt(_planGenerating: boolean, planReceived: boolean): boolean {
  return !planReceived;
}

export function shouldSeparatePlanningAndWaiting(planGenerating: boolean, planReceived: boolean): boolean {
  return planGenerating && shouldShowWaitingPrompt(planGenerating, planReceived);
}

export function formatWaitingPromptText(waitingStatsSuffix?: string): string {
  const suffix = waitingStatsSuffix?.trim();
  return suffix ? ` Waiting for response... ${suffix}` : " Waiting for response...";
}

function todoStatusSymbol(status: RuntimeTodoItem["status"]): string {
  switch (status) {
    case "completed":
      return "✔";
    case "in_progress":
      return "◼";
    default:
      return "◻";
  }
}

export const TODO_FIRST_PREFIX = "  ⎿  ";
export const TODO_NEXT_PREFIX = "     ";

export function formatTodoListLines(items: RuntimeTodoItem[], maxWidth: number): string[] {
  const result: string[] = [];
  items.forEach((item, index) => {
    const prefix = index === 0 ? TODO_FIRST_PREFIX : TODO_NEXT_PREFIX;
    const marker = `${todoStatusSymbol(item.status)} `;
    const contentWidth = Math.max(1, maxWidth - prefix.length - marker.length);
    const wrapped = wrapText(item.content, contentWidth).split("\n");
    wrapped.forEach((line, lineIndex) => {
      result.push(`${lineIndex === 0 ? prefix : TODO_NEXT_PREFIX}${lineIndex === 0 ? marker : "  "}${line}`);
    });
  });
  return result;
}

export function formatPlanBorderLine(maxWidth: number, title?: string): string {
  const width = Math.max(12, Math.floor(maxWidth));
  if (!title) return "─".repeat(width);

  const label = ` ${title} `;
  const labelWidth = stringWidth(label);
  const fillWidth = Math.max(2, width - labelWidth);
  const leftWidth = Math.floor(fillWidth / 2);
  const rightWidth = fillWidth - leftWidth;
  return `${"─".repeat(leftWidth)}${label}${"─".repeat(rightWidth)}`;
}

export function formatPlanFrameLines(content: string, maxWidth: number): string[] {
  const width = Math.max(12, Math.floor(maxWidth));
  const contentWidth = Math.max(1, width - 2);
  const renderedLines = renderMarkdownLines(content, contentWidth, false, true);
  return [
    formatPlanBorderLine(maxWidth, "Plan"),
    ...renderedLines.map((line) => `  ${line}`),
    formatPlanBorderLine(maxWidth),
  ];
}

function parentToolEntryExists(entries: TimelineEntry[], parentId: string): boolean {
  return entries.some((e) => e.kind === "tool" && e.toolCallId === parentId);
}

function buildChildrenByParent(entries: TimelineEntry[]): Map<string, TimelineEntry[]> {
  const m = new Map<string, TimelineEntry[]>();
  for (const e of entries) {
    if (e.kind !== "tool" || !e.parentToolCallId) {
      continue;
    }
    const id = e.parentToolCallId;
    const list = m.get(id) ?? [];
    list.push(e);
    m.set(id, list);
  }
  return m;
}

function nestedToolCallIdsToSkip(entries: TimelineEntry[]): Set<string> {
  const skip = new Set<string>();
  for (const e of entries) {
    if (e.kind !== "tool" || !e.toolCallId || !e.parentToolCallId) {
      continue;
    }
    if (parentToolEntryExists(entries, e.parentToolCallId)) {
      skip.add(e.toolCallId);
    }
  }
  return skip;
}

export type TimelineDisplayRow = {
  entry: TimelineEntry;
  nestedTools?: TimelineEntry[];
};

export function buildTimelineDisplayRows(entries: TimelineEntry[]): TimelineDisplayRow[] {
  const skip = nestedToolCallIdsToSkip(entries);
  const childrenByParent = buildChildrenByParent(entries);
  const rows: TimelineDisplayRow[] = [];
  for (const e of entries) {
    if (!SHOW_CLI_THINKING && e.kind === "thinking") {
      continue;
    }
    if (e.kind === "tool" && e.toolCallId && skip.has(e.toolCallId)) {
      continue;
    }
    let nestedTools: TimelineEntry[] | undefined;
    if (e.kind === "tool" && e.toolCallId) {
      const list = childrenByParent.get(e.toolCallId);
      if (list && list.length > 0) {
        nestedTools = list;
      }
    }
    rows.push({ entry: e, nestedTools });
  }
  return rows;
}
