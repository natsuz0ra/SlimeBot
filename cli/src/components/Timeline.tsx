/**
 * Timeline - message timeline component.
 * Renders user/assistant/system/tool-call entries in a single chronological view.
 */

import React, { useMemo } from "react";
import { Box, Text } from "ink";
import { renderMarkdownLines } from "../utils/markdownRenderer.js";
import { DOT } from "../utils/terminal.js";
import { stringWidth } from "../utils/stringWidth.js";
import type { TimelineEntry, ToolCallStatus } from "../types.js";
import {
  formatToolInvocation,
  wrapText,
  formatCollapsedLines,
  TOOL_OUTPUT_PREVIEW_LINES,
  formatToolExecutionOutput,
  formatToolParamEntries,
  formatAskQuestionsDisplay,
  formatAskQuestionsPending,
} from "../utils/format.js";
import { GradientFlowText } from "./GradientFlowText.js";
import { Markdown, StreamingMarkdown } from "./Markdown.js";
import { Spinner } from "./Spinner.js";

export const PLAN_GOLD = "#f59e0b";

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

function formatSubagentThinkingLines(entry: TimelineEntry, maxWidth: number): string[] {
  if (!entry.subagentThinking) return [];
  const thinking = entry.subagentThinking;
  const done = thinking.thinkingDone;
  const duration = done && thinking.thinkingDurationMs !== undefined
    ? (thinking.thinkingDurationMs / 1000).toFixed(1) + "s"
    : "";
  const label = done ? `Sub-agent thought for ${duration}` : "Sub-agent thinking...";
  const lines = [`   ${label}`];
  if (done && thinking.content.trim() !== "") {
    const wrapped = wrapText(thinking.content, Math.max(20, maxWidth - 6));
    lines.push(...wrapped.split("\n").map((line) => `     ${line}`));
  }
  return lines;
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
  let qaDisplayLines: string[] | null = null;
  if (isAskQuestions && (status === "completed" || status === "error" || status === "rejected")) {
    qaDisplayLines = formatAskQuestionsDisplay((entry.output || entry.content) || "");
  } else if (isAskQuestions && (status === "pending" || status === "executing")) {
    const questionsRaw = entry.params?.questions;
    if (questionsRaw) qaDisplayLines = formatAskQuestionsPending(questionsRaw);
  }
  const paramLines = qaDisplayLines ? [] : formatToolParamLines(entry, maxWidth);
  const resultLines = qaDisplayLines ? [] : (status === "completed" || status === "error" || status === "rejected")
    ? formatToolOutputLines(entry, maxWidth, toolOutputExpanded)
    : [];
  const subStreamLines =
    entry.subagentStream && entry.subagentStream.trim() !== ""
      ? formatSubagentStreamLines(entry.subagentStream, maxWidth, toolOutputExpanded)
      : [];
  const subThinkingLines = formatSubagentThinkingLines(entry, maxWidth);

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
              <GradientFlowText
                text=" Waiting for response..."
                enabled={true}
              />
            </Box>
          )}
        </>
      )}
    </Box>
  );
}
