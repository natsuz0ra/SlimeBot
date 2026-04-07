/**
 * Timeline - message timeline component.
 * Renders user/assistant/system/tool-call entries in a single chronological view.
 */

import React, { useMemo } from "react";
import { Box, Text } from "ink";
import { renderMarkdownLines } from "../utils/markdownRenderer.js";
import { DOT } from "../utils/terminal.js";
import type { TimelineEntry, ToolCallStatus } from "../types.js";
import { formatToolInvocation, truncateText, wrapText, formatCollapsedLines, TOOL_OUTPUT_PREVIEW_LINES } from "../utils/format.js";
import { GradientFlowText } from "./GradientFlowText.js";

interface TimelineProps {
  entries: TimelineEntry[];
  blinkOn: boolean;
  streaming: boolean;
  assistantWaiting: boolean;
  liveAssistant: string;
  maxWidth: number;
  compact: boolean;
  toolOutputExpanded: boolean;
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

export function formatToolOutputLines(entry: TimelineEntry, maxWidth: number, expanded: boolean): string[] {
  const outputPrefix = "   ⎿ ";
  const continuationPrefix = "     ";
  const prefixWidth = 5;
  const contentWidth = Math.max(1, maxWidth - prefixWidth);
  const raw = (entry.status === "error" || entry.status === "rejected")
    ? (entry.error || entry.content)
    : (entry.output || entry.content);
  const { lines: rawLines } = formatCollapsedLines(raw || "", TOOL_OUTPUT_PREVIEW_LINES, expanded);
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

function TimelineBlock({
  entry,
  blinkOn,
  maxWidth,
  compact,
  toolOutputExpanded,
}: {
  entry: TimelineEntry;
  blinkOn: boolean;
  maxWidth: number;
  compact: boolean;
  toolOutputExpanded: boolean;
}): React.ReactElement {
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

  const status = (entry.status || "completed") as ToolCallStatus;
  const dot = toolDotState(status);
  const invocation = formatToolInvocation(
    entry.toolName || "",
    entry.command || "",
    entry.params,
  );
  const resultLines = (status === "completed" || status === "error" || status === "rejected")
    ? formatToolOutputLines(entry, maxWidth, toolOutputExpanded)
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
}: TimelineProps): React.ReactElement {
  return (
    <Box flexDirection="column">
      {entries.map((entry, index) => (
        <React.Fragment key={`${entry.kind}-${entry.toolCallId || index}`}>
          {index > 0 && <Text> </Text>}
          <TimelineBlock entry={entry} blinkOn={blinkOn} maxWidth={maxWidth} compact={compact} toolOutputExpanded={toolOutputExpanded} />
        </React.Fragment>
      ))}

      {streaming && (
        <>
          {entries.length > 0 && <Text> </Text>}
          {assistantWaiting ? (
            <GradientFlowText
              text={`${DOT} Waiting for response...`}
              enabled={true}
            />
          ) : liveAssistant ? (
            <StreamingMarkdown content={liveAssistant} maxWidth={maxWidth} compact={compact} />
          ) : null}
        </>
      )}
    </Box>
  );
}

