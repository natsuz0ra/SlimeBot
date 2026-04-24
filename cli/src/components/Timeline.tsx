/**
 * Timeline - message timeline component.
 * Renders user/assistant/system/tool-call entries in a single chronological view.
 */

import React, { useMemo } from "react";
import { Box, Text } from "ink";
import { renderMarkdownLines } from "../utils/markdownRenderer.js";
import { DOT, stripAnsi } from "../utils/terminal.js";
import { stringWidth } from "../utils/stringWidth.js";
import type { TimelineEntry, ToolCallStatus } from "../types.js";
import {
  formatToolInvocation,
  wrapText,
  formatCollapsedLines,
  TOOL_OUTPUT_PREVIEW_LINES,
  formatToolExecutionOutput,
  formatToolParamEntries,
} from "../utils/format.js";
import { GradientFlowText } from "./GradientFlowText.js";
import { Spinner } from "./Spinner.js";

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
  const paramPrefix = "   :: ";
  const continuationPrefix = "      ";
  const prefixWidth = paramPrefix.length;
  const contentWidth = Math.max(1, maxWidth - prefixWidth);
  const rawLines = formatToolParamEntries(entry.params);
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

function StreamingMarkdown({
  content,
  maxWidth,
  compact,
}: {
  content: string;
  maxWidth: number;
  compact: boolean;
}): React.ReactElement {
  const contentWidth = Math.max(1, maxWidth - 2);
  const lines = useMemo(
    () => renderMarkdownLines(content, contentWidth, compact),
    [content, contentWidth, compact],
  );

  return (
    <Box flexDirection="column">
      {lines.map((line, index) => (
        <Text key={`${content}-${index}`}>
          {index === 0 ? (
            <Text bold color="white">
              {DOT}{" "}
            </Text>
          ) : (
            <Text>{"  "}</Text>
          )}
          <Text>{line}</Text>
        </Text>
      ))}
    </Box>
  );
}

function padVisible(content: string, targetWidth: number): string {
  const visible = stringWidth(stripAnsi(content));
  return `${content}${" ".repeat(Math.max(0, targetWidth - visible))}`;
}

export function formatPlanBlockLines(content: string, maxWidth: number): string[] {
  const outerWidth = Math.max(12, Math.floor(maxWidth) - 2);
  const contentWidth = Math.max(1, outerWidth - 4);
  const renderedLines = renderMarkdownLines(content, contentWidth, false, true);
  const title = " Plan ";
  const topPrefix = `╭─${title}`;
  const topFill = "─".repeat(Math.max(0, outerWidth - stringWidth(topPrefix) - 1));
  const lines = [`${topPrefix}${topFill}╮`];

  for (const line of renderedLines) {
    lines.push(`│ ${padVisible(line, contentWidth)} │`);
  }

  lines.push(`╰${"─".repeat(Math.max(0, outerWidth - 2))}╯`);
  return lines;
}

function PlanBlock({ content, maxWidth }: { content: string; maxWidth: number }): React.ReactElement {
  const lines = useMemo(() => formatPlanBlockLines(content, maxWidth), [content, maxWidth]);
  const borderColor = "#22d3ee";

  return (
    <Box flexDirection="column">
      {lines.map((line, i) => {
        const isBody = i > 0 && i < lines.length - 1;
        if (!isBody) {
          return (
            <Text key={i} color={borderColor} bold={i === 0}>
              {line}
            </Text>
          );
        }
        return (
          <Text key={i}>
            <Text color={borderColor}>{"│ "}</Text>
            <Text>{line.slice(2, -2)}</Text>
            <Text color={borderColor}>{" │"}</Text>
          </Text>
        );
      })}
    </Box>
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
}: {
  entry: TimelineEntry;
  blinkOn: boolean;
  maxWidth: number;
  compact: boolean;
  toolOutputExpanded: boolean;
  nestedUnderParent?: boolean;
  thinkingNumber?: number;
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
    return <StreamingMarkdown content={entry.content} maxWidth={maxWidth} compact={compact} />;
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
  const paramLines = formatToolParamLines(entry, maxWidth);
  const resultLines = (status === "completed" || status === "error" || status === "rejected")
    ? formatToolOutputLines(entry, maxWidth, toolOutputExpanded)
    : [];
  const subStreamLines =
    entry.subagentStream && entry.subagentStream.trim() !== ""
      ? formatSubagentStreamLines(entry.subagentStream, maxWidth, toolOutputExpanded)
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
      {paramLines.map((line, index) => (
        <Text key={`${entry.toolCallId || invocation}-param-${index}`} color="gray">
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
          />
          {row.nestedTools && row.nestedTools.length > 0 ? (
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
              <StreamingMarkdown content={liveAssistant} maxWidth={maxWidth} compact={compact} />
              <Text> </Text>
            </>
          )}
          {!planReceived && (
            <Box key="waiting">
              <Spinner enabled={true} />
              <GradientFlowText
                text={planGenerating ? " Planning..." : " Waiting for response..."}
                enabled={true}
              />
            </Box>
          )}
        </>
      )}
    </Box>
  );
}
