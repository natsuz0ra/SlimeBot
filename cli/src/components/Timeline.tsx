/**
 * Timeline - message timeline component.
 * Renders user/assistant/system/tool-call entries in a single chronological view.
 */

import React, { useMemo } from "react";
import { Box, Text } from "ink";
import { renderMarkdownLines } from "../utils/markdownRenderer.js";
import { DOT } from "../utils/terminal.js";
import { stringWidth } from "../utils/stringWidth.js";
import type { RuntimeTodoItem, TimelineEntry, ToolCallStatus } from "../types.js";
import {
  formatToolInvocation,
  wrapText,
  formatCollapsedLines,
  TOOL_OUTPUT_PREVIEW_LINES,
  formatToolExecutionOutput,
  formatToolTextValue,
  formatToolParamEntries,
  formatAskQuestionsDisplay,
  formatAskQuestionsPending,
  truncateText,
} from "../utils/format.js";
import { GradientFlowText } from "./GradientFlowText.js";
import { Markdown, StreamingMarkdown } from "./Markdown.js";
import { Spinner } from "./Spinner.js";

export const PLAN_GOLD = "#f59e0b";
export const WAITING_STATS_COLOR = "#64748b";

interface TimelineProps {
  entries: TimelineEntry[];
  blinkOn: boolean;
  streaming: boolean;
  assistantWaiting: boolean;
  liveAssistant: string;
  maxWidth: number;
  compact: boolean;
  toolOutputExpanded: boolean;
  thinkingEntryIndex: number;
  planGenerating: boolean;
  planReceived: boolean;
  waitingStatsSuffix?: string;
  runtimeTodos?: RuntimeTodoItem[];
}

function toolDotState(status: ToolCallStatus): { color: string; blink: boolean } {
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

function renderToolSuffix(status: ToolCallStatus): string {
  switch (status) {
    case "pending":
      return " pending approval...";
    case "executing":
      return " executing...";
    case "completed":
      return " completed";
    case "error":
      return " failed";
    case "rejected":
      return " rejected";
    default:
      return "";
  }
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
  const formatted = formatToolExecutionOutput(entry.toolName || "", entry.command || "", raw || "");
  const { lines: rawLines } = formatCollapsedLines(formatted, TOOL_OUTPUT_PREVIEW_LINES, expanded);
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

export function formatToolParamLines(entry: TimelineEntry, maxWidth: number): string[] {
  return formatToolParamLinesForParams(entry.params, maxWidth);
}

function formatToolParamLinesForParams(params: Record<string, string> | undefined, maxWidth: number): string[] {
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

function isRunSubagentEntry(entry: TimelineEntry): boolean {
  return (entry.toolName || "").trim().toLowerCase() === "run_subagent";
}

function displayParamValue(params: Record<string, string> | undefined, key: string): string {
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

function runSubagentExtraParams(params: Record<string, string> | undefined): Record<string, string> | undefined {
  if (!params) return undefined;
  const filtered: Record<string, string> = {};
  for (const [key, value] of Object.entries(params)) {
    const normalized = key.trim().toLowerCase();
    if (normalized === "context" || normalized === "task") continue;
    filtered[key] = value;
  }
  return Object.keys(filtered).length > 0 ? filtered : undefined;
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
      const wrapped = wrapText(item.summary, Math.max(1, maxWidth - summaryPrefix.length)).split("\n");
      result.push(`${summaryPrefix}${wrapped[0] ?? ""}`);
      const continuationPrefix = " ".repeat(summaryPrefix.length);
      for (const part of wrapped.slice(1)) {
        result.push(`${continuationPrefix}${part}`);
      }
    } else {
      result.push(...wrapText(summaryPrefix, maxWidth).split("\n"));
    }

    const detailPrefix = `   ${treeDetailPrefix(i, items.length)}  `;
    for (const detail of item.detailLines) {
      const wrapped = wrapText(detail, Math.max(1, maxWidth - detailPrefix.length));
      result.push(...wrapped.split("\n").map((part) => `${detailPrefix}${part}`));
    }
  }
  return result;
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
  const task = displayParamValue(entry.params, "task");
  const lines: string[] = [];

  lines.push(...formatSubagentSectionLines("Context", context, maxWidth, expanded));
  lines.push(...formatSubagentSectionLines("Task", task, maxWidth, expanded));

  const thinkingLabel = entry.subagentThinking
    ? (entry.subagentThinking.thinkingDone
      ? `thinking complete${entry.subagentThinking.thinkingDurationMs !== undefined ? ` in ${(entry.subagentThinking.thinkingDurationMs / 1000).toFixed(1)}s` : ""}`
      : "Sub-agent thinking...")
    : "";
  const toolCount = nestedTools.length;
  const thinkingToolsSummary = [
    thinkingLabel,
    toolCount > 0 ? `${toolCount} tool${toolCount === 1 ? "" : "s"}` : "",
  ].filter(Boolean).join(" · ") || "No thinking or tools yet";
  lines.push(`   Thinking & tools: ${thinkingToolsSummary}${expanded ? " (ctrl+o to collapse)" : " (ctrl+o to expand)"}`);

  if (expanded) {
    lines.push(...formatSubagentThinkingLines(entry, maxWidth, true));
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

export function shouldShowWaitingPrompt(planGenerating: boolean, planReceived: boolean): boolean {
  return !planReceived;
}

export function shouldSeparatePlanningAndWaiting(planGenerating: boolean, planReceived: boolean): boolean {
  return planGenerating && shouldShowWaitingPrompt(planGenerating, planReceived);
}

export function formatWaitingPromptText(waitingStatsSuffix?: string): string {
  const suffix = waitingStatsSuffix?.trim();
  return suffix ? ` Waiting for response... ${suffix}` : " Waiting for response...";
}

function WaitingPrompt({ waitingStatsSuffix }: { waitingStatsSuffix?: string }): React.ReactElement {
  const suffix = waitingStatsSuffix?.trim();
  return (
    <Text>
      <GradientFlowText
        text=" Waiting for response..."
        enabled={true}
      />
      {suffix ? (
        <Text color={WAITING_STATS_COLOR}>{` ${suffix}`}</Text>
      ) : null}
    </Text>
  );
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

export function formatTodoListLines(items: RuntimeTodoItem[], maxWidth: number): string[] {
  const result: string[] = [];
  const firstPrefix = "  ⎿  ";
  const nextPrefix = "     ";
  items.forEach((item, index) => {
    const prefix = index === 0 ? firstPrefix : nextPrefix;
    const marker = `${todoStatusSymbol(item.status)} `;
    const contentWidth = Math.max(1, maxWidth - prefix.length - marker.length);
    const wrapped = wrapText(item.content, contentWidth).split("\n");
    wrapped.forEach((line, lineIndex) => {
      result.push(`${lineIndex === 0 ? prefix : nextPrefix}${lineIndex === 0 ? marker : "  "}${line}`);
    });
  });
  return result;
}

function TodoList({ items, maxWidth }: { items: RuntimeTodoItem[]; maxWidth: number }): React.ReactElement | null {
  if (items.length === 0) return null;
  return (
    <Box flexDirection="column">
      {formatTodoListLines(items, maxWidth).map((line, index) => (
        <Text key={`todo-${index}`} color={index === 0 ? "gray" : undefined}>{line}</Text>
      ))}
    </Box>
  );
}

function formatPlanBorderLine(maxWidth: number, title?: string): string {
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

function PlanBlock({ content, maxWidth }: { content: string; maxWidth: number }): React.ReactElement {
  const topBorder = useMemo(() => formatPlanBorderLine(maxWidth, "Plan"), [maxWidth]);
  const bottomBorder = useMemo(() => formatPlanBorderLine(maxWidth), [maxWidth]);
  const contentWidth = Math.max(1, Math.max(12, Math.floor(maxWidth)) - 2);

  return (
    <Box flexDirection="column">
      <Text color={PLAN_GOLD} bold>{topBorder}</Text>
      <Box marginLeft={2}>
        <Markdown
          content={content}
          maxWidth={contentWidth}
          compact={false}
          preserveTrailingBlanks
        />
      </Box>
      <Text color={PLAN_GOLD}>{bottomBorder}</Text>
    </Box>
  );
}

function PlanningIndicator({ blinkOn }: { blinkOn: boolean }): React.ReactElement {
  const parts = formatPlanningIndicatorParts(blinkOn);
  return (
    <Text>
      <Text bold color={parts.color}>{parts.dot}</Text>
      <Text>{" "}</Text>
      <Text color={parts.color} bold>{parts.label}</Text>
    </Text>
  );
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

type TimelineDisplayRow = {
  entry: TimelineEntry;
  nestedTools?: TimelineEntry[];
};

function buildTimelineDisplayRows(entries: TimelineEntry[]): TimelineDisplayRow[] {
  const skip = nestedToolCallIdsToSkip(entries);
  const childrenByParent = buildChildrenByParent(entries);
  const rows: TimelineDisplayRow[] = [];
  for (const e of entries) {
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

function TimelineBlock({
  entry,
  blinkOn,
  maxWidth,
  compact,
  toolOutputExpanded,
  nestedUnderParent,
  thinkingNumber,
  nestedTools,
}: {
  entry: TimelineEntry;
  blinkOn: boolean;
  maxWidth: number;
  compact: boolean;
  toolOutputExpanded: boolean;
  nestedUnderParent?: boolean;
  thinkingNumber?: number;
  nestedTools?: TimelineEntry[];
}): React.ReactElement {
  if (entry.kind === "plan") {
    return <PlanBlock content={entry.content} maxWidth={maxWidth} />;
  }

  if (entry.kind === "user") {
    const lines = entry.content.split("\n");
    return (
      <Box flexDirection="column">
        {lines.map((line, i) => (
          <Text key={i}>
            {i === 0 ? (
              <Text color="cyan">{"\u276F "}</Text>
            ) : (
              <Text>{"  "}</Text>
            )}
            <Text>{line}</Text>
          </Text>
        ))}
      </Box>
    );
  }

  if (entry.kind === "assistant") {
    return (
      <StreamingMarkdown
        content={entry.content}
        maxWidth={Math.max(1, maxWidth - 2)}
        compact={compact}
        renderPrefix={(index) =>
          index === 0 ? (
            <Text bold color="white">
              {DOT}{" "}
            </Text>
          ) : (
            <Text>{"  "}</Text>
          )
        }
      />
    );
  }

  if (entry.kind === "system") {
    return (
      <Text>
        <Text bold color="yellow">
          {DOT}{" "}
        </Text>
        <Text>{entry.content}</Text>
      </Text>
    );
  }

  if (entry.kind === "thinking") {
    const done = entry.thinkingDone;
    const label = formatThinkingLabel(entry);
    const numPrefix = thinkingNumber !== undefined ? `[${thinkingNumber}] ` : "";
    const dotColor = done ? "#38bdf8" : "#22d3ee";
    const labelColor = done ? "#7dd3fc" : "#22d3ee";
    const indexColor = "#67e8f9";
    return (
      <Text>
        <Text bold color={dotColor}>
          {!done && !blinkOn ? " " : DOT}
        </Text>
        <Text>{" "}</Text>
        <Text color={indexColor}>{numPrefix}</Text>
        <Text color={labelColor} bold={!done}>{label}</Text>
      </Text>
    );
  }

  const status = (entry.status || "completed") as ToolCallStatus;
  const dot = toolDotState(status);
  const nestPrefix =
    entry.parentToolCallId !== undefined && entry.parentToolCallId !== ""
      ? nestedUnderParent
        ? "  "
        : "> "
      : "";
  const invocation =
    nestPrefix +
    formatToolInvocation(
      entry.toolName || "",
      entry.command || "",
    );
  const isAskQuestions = (entry.toolName || "").trim().toLowerCase() === "ask_questions";
  const isRunSubagent = isRunSubagentEntry(entry);
  let qaDisplayLines: string[] | null = null;
  if (isAskQuestions && (status === "completed" || status === "error" || status === "rejected")) {
    qaDisplayLines = formatAskQuestionsDisplay((entry.output || entry.content) || "");
  } else if (isAskQuestions && (status === "pending" || status === "executing")) {
    const questionsRaw = entry.params?.questions;
    if (questionsRaw) qaDisplayLines = formatAskQuestionsPending(questionsRaw);
  }
  const paramLines = qaDisplayLines || isRunSubagent ? [] : formatToolParamLines(entry, maxWidth);
  const resultLines = qaDisplayLines ? [] : (status === "completed" || status === "error" || status === "rejected")
    ? formatToolOutputLines(entry, maxWidth, toolOutputExpanded)
    : [];
  const subStreamLines =
    entry.subagentStream && entry.subagentStream.trim() !== ""
      ? formatSubagentStreamLines(entry.subagentStream, maxWidth, toolOutputExpanded)
      : [];
  const subThinkingLines = formatSubagentThinkingLines(entry, maxWidth, toolOutputExpanded);
  const runSubagentLines = isRunSubagent
    ? formatRunSubagentDetailLines(entry, nestedTools || [], maxWidth, toolOutputExpanded)
    : [];

  return (
    <Box flexDirection="column">
      <Text>
        <Text bold color={dot.color}>
          {dot.blink && !blinkOn ? " " : DOT}
        </Text>
        <Text>{" "}</Text>
        <Text>{invocation}{renderToolSuffix(status)}</Text>
      </Text>
      {qaDisplayLines ? (
        qaDisplayLines.map((line, index) => (
          <Text key={`${entry.toolCallId || invocation}-qa-${index}`} color="gray">
            {"   "}{line}
          </Text>
        ))
      ) : isRunSubagent ? (
        runSubagentLines.map((line, index) => (
          <Text key={`${entry.toolCallId || invocation}-run-subagent-${index}`} color={line.includes("Thinking & tools") ? "cyan" : "gray"}>
            {line}
          </Text>
        ))
      ) : (
        <>
          {paramLines.map((line, index) => (
            <Text key={`${entry.toolCallId || invocation}-param-${index}`} color="gray">
              {line}
            </Text>
          ))}
          {subThinkingLines.map((line, index) => (
            <Text key={`${entry.toolCallId || invocation}-subthink-${index}`} color="cyan">
              {line}
            </Text>
          ))}
          {subStreamLines.map((line, index) => (
            <Text key={`${entry.toolCallId || invocation}-sub-${index}`} color="gray">
              {line}
            </Text>
          ))}
          {resultLines.map((line, index) => (
            <Text key={`${entry.toolCallId || invocation}-result-${index}`}>{line}</Text>
          ))}
        </>
      )}
    </Box>
  );
}

export function Timeline({
  entries,
  blinkOn,
  streaming,
  assistantWaiting,
  liveAssistant,
  maxWidth,
  compact,
  toolOutputExpanded,
  thinkingEntryIndex,
  planGenerating,
  planReceived,
  waitingStatsSuffix,
  runtimeTodos = [],
}: TimelineProps): React.ReactElement {
  const displayRows = useMemo(() => buildTimelineDisplayRows(entries), [entries]);
  let thinkingCounter = 0;

  return (
    <Box flexDirection="column">
      {displayRows.map((row, index) => (
        <React.Fragment key={`${row.entry.kind}-${row.entry.toolCallId ?? `r-${index}`}`}>
          {index > 0 && <Text> </Text>}
          <TimelineBlock
            entry={row.entry}
            blinkOn={blinkOn}
            maxWidth={maxWidth}
            compact={compact}
            toolOutputExpanded={toolOutputExpanded}
            thinkingNumber={row.entry.kind === "thinking" ? ++thinkingCounter : undefined}
            nestedTools={row.nestedTools}
          />
          {row.nestedTools && row.nestedTools.length > 0 && !isRunSubagentEntry(row.entry) ? (
            <Box flexDirection="column" marginLeft={2}>
              {row.nestedTools.map((child, ci) => (
                <React.Fragment key={`nested-${child.toolCallId ?? ci}`}>
                  {ci > 0 && <Text> </Text>}
                  <TimelineBlock
                    entry={child}
                    blinkOn={blinkOn}
                    maxWidth={Math.max(10, maxWidth - 2)}
                    compact={compact}
                    toolOutputExpanded={toolOutputExpanded}
                    nestedUnderParent
                  />
                </React.Fragment>
              ))}
            </Box>
          ) : null}
        </React.Fragment>
      ))}

      {streaming && (
        <>
          {entries.length > 0 && <Text> </Text>}
          {liveAssistant && !assistantWaiting && (
            <>
              <StreamingMarkdown
                content={liveAssistant}
                maxWidth={Math.max(1, maxWidth - 2)}
                compact={compact}
                renderPrefix={(index) =>
                  index === 0 ? (
                    <Text bold color="white">
                      {DOT}{" "}
                    </Text>
                  ) : (
                    <Text>{"  "}</Text>
                  )
                }
              />
              <Text> </Text>
            </>
          )}
          {planGenerating && !planReceived && (
            <PlanningIndicator blinkOn={blinkOn} />
          )}
          {shouldSeparatePlanningAndWaiting(planGenerating, planReceived) && <Text> </Text>}
          {shouldShowWaitingPrompt(planGenerating, planReceived) && (
            <Box key="waiting">
              <Spinner enabled={true} />
              <WaitingPrompt waitingStatsSuffix={waitingStatsSuffix} />
            </Box>
          )}
          {runtimeTodos.length > 0 && (
            <TodoList items={runtimeTodos} maxWidth={maxWidth} />
          )}
        </>
      )}
    </Box>
  );
}
