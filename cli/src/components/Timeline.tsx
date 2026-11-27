/**
 * Timeline - message timeline component.
 * Renders user/assistant/system/tool-call entries in a single chronological view.
 */

import React, { useMemo } from "react";
import { Box, Text } from "ink";
import { CLI_ACCENT_COLOR, CLI_ACCENT_SOFT_COLOR, DOT } from "../utils/terminal.js";
import type { RuntimeTodoItem, TimelineEntry, ToolCallStatus } from "../types.js";
import {
  formatToolCallSummary,
  wrapText,
  formatAskQuestionsDisplay,
  formatAskQuestionsPending,
  formatAskQuestionsQuestionsOnly,
} from "../utils/format.js";
import {
  PLAN_GOLD,
  SHOW_CLI_THINKING,
  TODO_FIRST_PREFIX,
  TODO_NEXT_PREFIX,
  TOOL_SUMMARY_TAG_COLOR,
  WAITING_STATS_COLOR,
  buildTimelineDisplayRows,
  formatPlanBorderLine,
  formatPlanningIndicatorParts,
  formatRunSubagentDetailLines,
  formatLightweightToolGroupLines,
  formatLightweightToolGroupHeader,
  formatSubagentStreamLines,
  formatSubagentThinkingLines,
  formatThinkingLabel,
  formatToolOutputLines,
  formatToolParamLines,
  formatToolStatusPart,
  formatToolSummaryTag,
  formatTimelineToolInvocation,
  getRunSubagentDetailLineColor,
  isFileToolEntry,
  isRunSubagentEntry,
  shouldSeparatePlanningAndWaiting,
  shouldShowWaitingPrompt,
  toolDotState,
} from "../utils/timelineFormat.js";
import { GradientFlowText } from "./GradientFlowText.js";
import { FileToolDiffBlock } from "./FileToolDiffBlock.js";
import { Markdown, StreamingMarkdown } from "./Markdown.js";
import { Spinner } from "./Spinner.js";
import { formatAgentTeamRows } from "../utils/agentTeam.js";

interface TimelineProps {
  entries: TimelineEntry[];
  blinkOn: boolean;
  streaming: boolean;
  assistantWaiting: boolean;
  liveAssistant: string;
  maxWidth: number;
  compact: boolean;
  toolOutputExpanded: boolean;
  planGenerating: boolean;
  planReceived: boolean;
  waitingStatsSuffix?: string;
  runtimeTodos?: RuntimeTodoItem[];
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

function TodoList({ items, maxWidth }: { items: RuntimeTodoItem[]; maxWidth: number }): React.ReactElement | null {
  if (items.length === 0) return null;
  return (
    <Box flexDirection="column">
      {items.flatMap((item, itemIndex) => {
        const prefix = itemIndex === 0 ? TODO_FIRST_PREFIX : TODO_NEXT_PREFIX;
        const marker = `${item.status === "completed" ? "✔" : item.status === "in_progress" ? "◼" : "◻"} `;
        const contentWidth = Math.max(1, maxWidth - prefix.length - marker.length);
        return wrapText(item.content, contentWidth).split("\n").map((line, lineIndex) => (
          <Text key={`todo-${item.id}-${lineIndex}`}>
            <Text>{lineIndex === 0 ? prefix : TODO_NEXT_PREFIX}</Text>
            <Text>{lineIndex === 0 ? marker : "  "}</Text>
            <Text strikethrough={item.status === "completed"}>{line}</Text>
          </Text>
        ));
      })}
    </Box>
  );
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
    const contentWidth = Math.max(1, maxWidth - 2);
    const lines = entry.content
      .split("\n")
      .flatMap((line) => wrapText(line, contentWidth).split("\n"));
    return (
      <Box flexDirection="column">
        {lines.map((line, i) => (
          <Text key={i}>
            {i === 0 ? (
              <Text color={CLI_ACCENT_COLOR}>{"\u276F "}</Text>
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
            <Text bold>
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
    const isError = entry.content.startsWith("Error:") || entry.content.startsWith("Failed ");
    return (
      <Text>
        <Text bold color={isError ? "red" : CLI_ACCENT_COLOR}>
          {isError ? "⚠ " : "ℹ "}
        </Text>
        <Text color={isError ? "red" : undefined} dimColor={!isError}>{entry.content}</Text>
      </Text>
    );
  }

  if (entry.kind === "thinking") {
    const done = entry.thinkingDone;
    const label = formatThinkingLabel(entry);
    const numPrefix = thinkingNumber !== undefined ? `[${thinkingNumber}] ` : "";
    const dotColor = done ? CLI_ACCENT_SOFT_COLOR : CLI_ACCENT_COLOR;
    const labelColor = done ? CLI_ACCENT_SOFT_COLOR : CLI_ACCENT_COLOR;
    const indexColor = CLI_ACCENT_COLOR;
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
    formatTimelineToolInvocation(entry.toolName, entry.command);
  const summaryParams = entry.subagentTitle
    ? { ...(entry.params || {}), title: entry.subagentTitle }
    : entry.params;
  const normalizedTool = (entry.toolName || "").trim().toLowerCase();
  const normalizedCommand = (entry.command || "").trim().toLowerCase();
  const isExecRun = normalizedTool === "exec" && normalizedCommand === "run";
  const summaryParamsForDisplay = isExecRun && !toolOutputExpanded && status === "completed"
    ? (() => {
        const cloned = { ...(summaryParams || {}) } as Record<string, unknown>;
        delete cloned.command;
        return cloned;
      })()
    : summaryParams;
  const summaryTag = formatToolSummaryTag(
    formatToolCallSummary(entry.toolName || "", entry.command || "", summaryParamsForDisplay),
  );
  const statusPart = formatToolStatusPart(status);
  const isAskQuestions = (entry.toolName || "").trim().toLowerCase() === "ask_questions";
  const isRunSubagent = isRunSubagentEntry(entry);
  let qaDisplayLines: string[] | null = null;
  if (isAskQuestions && status === "completed") {
    qaDisplayLines = formatAskQuestionsDisplay((entry.output || entry.content) || "");
  } else if (isAskQuestions && (status === "error" || status === "rejected")) {
    qaDisplayLines = formatAskQuestionsDisplay((entry.output || entry.content) || "");
    if (!qaDisplayLines) {
      const questionsRaw = entry.params?.questions;
      if (questionsRaw) qaDisplayLines = formatAskQuestionsQuestionsOnly(String(questionsRaw));
    }
  } else if (isAskQuestions && (status === "pending" || status === "executing")) {
    const questionsRaw = entry.params?.questions;
    if (typeof questionsRaw === "string") qaDisplayLines = formatAskQuestionsPending(questionsRaw);
  }
  const isFileTool = isFileToolEntry(entry);
  const paramLines = qaDisplayLines || isRunSubagent || isFileTool ? [] : formatToolParamLines(entry, maxWidth, toolOutputExpanded);
  const resultLines = qaDisplayLines || isFileTool ? [] : (status === "completed" || status === "error" || status === "rejected")
    ? formatToolOutputLines(entry, maxWidth, toolOutputExpanded)
    : [];
  const subStreamLines =
    entry.subagentStream && entry.subagentStream.trim() !== ""
      ? formatSubagentStreamLines(entry.subagentStream, maxWidth, toolOutputExpanded)
      : [];
  const subThinkingLines = SHOW_CLI_THINKING
    ? formatSubagentThinkingLines(entry, maxWidth, toolOutputExpanded)
    : [];
  const runSubagentLines = isRunSubagent
    ? formatRunSubagentDetailLines(entry, nestedTools || [], maxWidth, toolOutputExpanded)
    : [];
  let runSubagentLineColor = "gray";
  const coloredRunSubagentLines = runSubagentLines.map((line) => {
    runSubagentLineColor = getRunSubagentDetailLineColor(line, runSubagentLineColor);
    return { line, color: runSubagentLineColor };
  });

  return (
    <Box flexDirection="column">
      <Text>
        <Text bold color={dot.color}>
          {dot.blink && !blinkOn ? " " : DOT}
        </Text>
        <Text>{" "}</Text>
        <Text bold>{invocation}</Text>
        {summaryTag ? (
          <Text color={TOOL_SUMMARY_TAG_COLOR}>{` ${summaryTag}`}</Text>
        ) : null}
        {statusPart.text ? (
          <Text color={statusPart.color}>{` ${statusPart.text}`}</Text>
        ) : null}
      </Text>
      {qaDisplayLines ? (
        qaDisplayLines.map((line, index) => (
          <Text key={`${entry.toolCallId || invocation}-qa-${index}`} color="gray">
            {"   "}{line}
          </Text>
        ))
      ) : isRunSubagent ? (
        coloredRunSubagentLines.map((item, index) => (
          <Text key={`${entry.toolCallId || invocation}-run-subagent-${index}`} color={item.color}>
            {item.line}
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
            <Text key={`${entry.toolCallId || invocation}-subthink-${index}`} color={CLI_ACCENT_COLOR}>
              {line}
            </Text>
          ))}
          {subStreamLines.map((line, index) => (
            <Text key={`${entry.toolCallId || invocation}-sub-${index}`} color="gray">
              {line}
            </Text>
          ))}
          {isFileTool ? (
            <FileToolDiffBlock
              entry={entry}
              maxWidth={maxWidth}
              expanded={toolOutputExpanded}
            />
          ) : null}
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
        <React.Fragment key={row.kind === "lightweight_tool_group" ? row.id : row.kind === "agent_team" ? row.team.id : `${row.entry.kind}-${row.entry.toolCallId ?? `r-${index}`}`}>
          {index > 0 && <Text> </Text>}
          {row.kind === "agent_team" ? (
            <Box flexDirection="column">
              {formatAgentTeamRows(row.team, maxWidth).map((line, lineIndex) => (
                <Text key={`${row.team.id}-line-${lineIndex}`} color={line.color}>
                  {line.active && !blinkOn ? line.text.replace(/^./u, " ") : line.text}
                </Text>
              ))}
            </Box>
          ) : row.kind === "lightweight_tool_group" ? (
            (() => {
              const runningOverride = streaming && row.trailing;
              const autoPreview = streaming && !toolOutputExpanded;
              const groupExpanded = toolOutputExpanded || autoPreview;
              const groupFormatOptions = {
                detailLimit: autoPreview ? 3 : undefined,
                tail: autoPreview,
                preview: autoPreview,
                statusOverride: runningOverride ? ("running" as const) : undefined,
              };
              const failed = row.items.some((item) => item.status === "error" || item.status === "rejected");
              const dot = toolDotState(runningOverride ? "executing" : failed ? "error" : "completed");
              const detailLines = groupExpanded
                ? formatLightweightToolGroupLines(row.items, maxWidth, true, groupFormatOptions).slice(1)
                : [];
              return (
                <Box flexDirection="column">
                  <Text>
                    <Text bold color={dot.color}>{dot.blink && !blinkOn ? " " : DOT}</Text>
                    <Text>{" "}</Text>
                    <Text bold>tools</Text>
                    <Text dimColor>{` ${formatLightweightToolGroupHeader(row.items, groupExpanded, groupFormatOptions)}`}</Text>
                  </Text>
                  {detailLines.map((line, lineIndex) => (
                    <Text key={`${row.id}-line-${lineIndex}`}>{line}</Text>
                  ))}
                </Box>
              );
            })()
          ) : (
            <TimelineBlock
              entry={row.entry}
              blinkOn={blinkOn}
              maxWidth={maxWidth}
              compact={compact}
              toolOutputExpanded={toolOutputExpanded}
              thinkingNumber={row.entry.kind === "thinking" ? ++thinkingCounter : undefined}
              nestedTools={row.nestedTools}
            />
          )}
          {row.kind === "entry" && row.nestedTools && row.nestedTools.length > 0 && !isRunSubagentEntry(row.entry) ? (
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
                    <Text bold>
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
