/**
 * ApprovalView - queue-style tool approval dialog.
 */

import React from "react";
import { Box, Text } from "ink";
import { filterToolParamsForDetail, formatToolCallSummary, truncateText, wrapText } from "../utils/format.js";
import { buildFileToolDisplay, isFileToolName } from "../utils/fileToolDisplay.js";
import { renderColorDiffRows } from "../native/colorDiff.js";
import type { ApprovalReviewItem, ApprovalReviewStatus } from "../types.js";

interface ApprovalItem {
  toolCallId: string;
  toolName: string;
  command: string;
  params: Record<string, unknown>;
}

interface ApprovalViewProps {
  toolName: string;
  command: string;
  params: Record<string, unknown>;
  items?: ApprovalItem[];
  approvalReviewItems?: ApprovalReviewItem[];
  cursor?: number;
  markedApprovalIds?: string[];
  columns?: number;
}

export interface ApprovalQueueRow {
  index: string;
  cursor: " " | "❯";
  mark: "☐" | "☑";
  toolLabel: string;
  riskLabel: "READ" | "WRITE" | "EXEC" | "TOOL";
  riskColor: "blue" | "yellow" | "magenta" | "gray";
  summary: string;
}

export interface ApprovalProgressDot {
  symbol: "●" | "○";
  color: "green" | "red" | "yellow" | "gray";
}

const DETAIL_PREVIEW_LINES = 5;
const FILE_DIFF_PREVIEW_LINES = 3;

function approvalItemsFromProps(props: Pick<ApprovalViewProps, "toolName" | "command" | "params" | "items">): ApprovalItem[] {
  return props.items && props.items.length > 0
    ? props.items
    : [{ toolCallId: "", toolName: props.toolName, command: props.command, params: props.params }];
}

export function formatApprovalTitle(currentIndex: number, totalCount: number): string {
  if (totalCount <= 0) return "Tool approval  0 / 0";
  return `Tool approval  ${Math.min(currentIndex + 1, Math.max(1, totalCount))} / ${totalCount}`;
}

function riskForTool(item: ApprovalItem): Pick<ApprovalQueueRow, "riskLabel" | "riskColor"> {
  const tool = item.toolName.trim().toLowerCase();
  if (tool === "file_read") return { riskLabel: "READ", riskColor: "blue" };
  if (tool === "file_write" || tool === "file_edit") return { riskLabel: "WRITE", riskColor: "yellow" };
  if (tool === "exec" && item.command.trim().toLowerCase() === "run") return { riskLabel: "EXEC", riskColor: "magenta" };
  return { riskLabel: "TOOL", riskColor: "gray" };
}

function toolLabel(item: ApprovalItem): string {
  const tool = item.toolName.trim() || "tool";
  const command = item.command.trim();
  return command ? `${tool}.${command}` : tool;
}

function compactSummary(item: ApprovalItem): string {
  const summary = formatToolCallSummary(item.toolName, item.command, item.params).trim();
  if (summary) return summary;
  if (item.toolName.trim().toLowerCase() === "exec" && item.command.trim().toLowerCase() === "run") {
    const command = String(item.params.command ?? "").trim();
    if (command) return command;
  }
  return "(no summary)";
}

export function buildApprovalQueueRows(
  items: ApprovalItem[],
  cursor: number,
  markedApprovalIds: string[],
): ApprovalQueueRow[] {
  const marked = new Set(markedApprovalIds);
  return items.map((item, index) => ({
    index: String(index + 1).padStart(2, "0"),
    cursor: index === cursor ? "❯" : " ",
    mark: marked.has(item.toolCallId) ? "☑" : "☐",
    toolLabel: toolLabel(item),
    ...riskForTool(item),
    summary: truncateText(compactSummary(item), 76),
  }));
}

export function buildApprovalProgressDots(
  items: Array<Pick<ApprovalReviewItem, "approvalStatus">>,
  currentIndex = 0,
): ApprovalProgressDot[] {
  return items.map((item, index) => {
    if (item.approvalStatus === "approved") return { symbol: "●", color: "green" };
    if (item.approvalStatus === "rejected") return { symbol: "●", color: "red" };
    if (index === currentIndex) return { symbol: "●", color: "yellow" };
    return { symbol: "○", color: "gray" };
  });
}

function stringifyParam(value: unknown): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function detailParamLines(item: ApprovalItem): string[] {
  if (item.toolName.trim().toLowerCase() === "ask_questions") {
    return [];
  }
  const params = filterToolParamsForDetail(item.toolName, item.command, item.params) || {};
  return Object.entries(params).map(([key, value]) => `${key}=${stringifyParam(value)}`);
}

function diffPreviewLines(item: ApprovalItem): string[] {
  if (!isFileToolName(item.toolName)) return [];
  const display = buildFileToolDisplay(item);
  if (!display) return [];
  const lines = [
    `File: ${display.filePath}`,
    `Operation: ${display.operation}`,
    `Summary: ${display.summary}`,
  ];
  const diffLines = display.diffLines.slice(0, DETAIL_PREVIEW_LINES);
  if (diffLines.length > 0) {
    lines.push("Diff preview:");
    for (const line of diffLines) {
      const prefix = line.kind === "added" ? "+" : line.kind === "removed" ? "-" : " ";
      lines.push(`${prefix} ${line.text}`);
    }
    if (display.diffLines.length > diffLines.length) {
      lines.push(`... +${display.diffLines.length - diffLines.length} folded lines`);
    }
  }
  return lines;
}

export function buildApprovalDetailLines(item: ApprovalItem | undefined): string[] {
  if (!item) return ["Selected details", "(no pending tool call)"];
  const lines = [
    "Selected details",
    `${toolLabel(item)} · ${compactSummary(item)}`,
  ];
  if (item.toolName.trim().toLowerCase() === "exec" && item.command.trim().toLowerCase() === "run") {
    const command = String(item.params.command ?? "").trim();
    if (command) {
      lines.push("Command", command);
    }
  }
  const fileLines = diffPreviewLines(item);
  if (fileLines.length > 0) {
    lines.push(...fileLines);
  }
  const params = detailParamLines(item);
  if (params.length > 0) {
    lines.push("Params", ...params);
  }
  return lines;
}

function detailLineColor(line: string): "cyan" | "gray" | "green" | "red" | "white" {
  if (line === "Selected details" || line === "Command" || line === "Params" || line === "Diff preview:") return "cyan";
  if (line.startsWith("+ ")) return "green";
  if (line.startsWith("- ")) return "red";
  if (line.startsWith("... ") || line === "(no pending tool call)") return "gray";
  return "white";
}

function reviewItemsFromApprovals(items: ApprovalItem[], reviewItems: ApprovalReviewItem[] | undefined): ApprovalReviewItem[] {
  if (reviewItems && reviewItems.length > 0) return reviewItems;
  return items.map((item) => ({ ...item, approvalStatus: "pending" as ApprovalReviewStatus }));
}

function currentReviewIndex(reviewItems: ApprovalReviewItem[], currentItem: ApprovalItem | undefined, fallbackIndex: number): number {
  if (!currentItem) return 0;
  const index = reviewItems.findIndex((item) => item.toolCallId === currentItem.toolCallId);
  return index >= 0 ? index : fallbackIndex;
}

function riskDescription(item: ApprovalItem): string {
  const tool = item.toolName.trim().toLowerCase();
  const command = item.command.trim().toLowerCase();
  if (tool === "exec" && command === "run") return "Runs a shell command in the current workspace.";
  if (tool === "file_write" || tool === "file_edit") return "Modifies files in the current workspace.";
  if (tool === "file_read") return "Reads files from the current workspace.";
  return "Requests permission before this tool runs.";
}

type DetailLine =
  | { kind: "field"; key: string; value: string; color?: "gray" | "white" }
  | { kind: "diff"; gutter: string; content: string }
  | { kind: "hint"; value: string }
  | { kind: "rule"; value: string };

function keyValueLines(item: ApprovalItem, width: number): DetailLine[] {
  const lines: DetailLine[] = [];
  if (item.toolName.trim().toLowerCase() === "exec" && item.command.trim().toLowerCase() === "run") {
    const command = String(item.params.command ?? "").trim();
    if (command) {
      lines.push({ kind: "field", key: "command", value: command });
    }
    const cwd = String(item.params.cwd ?? "").trim();
    if (cwd) {
      lines.push({ kind: "field", key: "cwd", value: cwd });
    }
    return lines;
  }

  if (isFileToolName(item.toolName)) {
    const display = buildFileToolDisplay(item);
    if (display) {
      lines.push({ kind: "field", key: "file", value: display.filePath });
      lines.push({ kind: "field", key: "change", value: display.summary });
      const diffLines = display.diffLines.slice(0, FILE_DIFF_PREVIEW_LINES);
      if (diffLines.length > 0) {
        const border = "─".repeat(Math.max(8, width));
        lines.push({ kind: "rule", value: border });
        const rows = renderColorDiffRows({
          filePath: display.filePath,
          lines: diffLines,
          width: Math.max(12, width),
        });
        for (const row of rows) {
          lines.push({ kind: "diff", gutter: row.gutter, content: row.content });
        }
        lines.push({ kind: "rule", value: border });
      }
      if (display.diffLines.length > diffLines.length) {
        lines.push({ kind: "hint", value: `... ${display.diffLines.length - diffLines.length} more changed lines` });
      }
      return lines;
    }
  }

  const params = detailParamLines(item).slice(0, 4);
  for (const param of params) {
    const [key, ...rest] = param.split("=");
    lines.push({ kind: "field", key: key || "param", value: rest.join("=") || param });
  }
  if (params.length === 0) {
    const summary = compactSummary(item);
    if (summary && summary !== "(no summary)") {
      lines.push({ kind: "field", key: "summary", value: summary });
    }
  }
  return lines.map((line) => line.kind !== "field" ? line : ({
    ...line,
    value: wrapText(line.value, Math.max(20, width - 12)).split("\n").join("\n"),
  }));
}

export function ApprovalView({
  toolName,
  command,
  params,
  items,
  approvalReviewItems,
  cursor = 0,
  columns = 80,
}: ApprovalViewProps): React.ReactElement {
  const approvalItems = approvalItemsFromProps({ toolName, command, params, items });
  const safeCursor = Math.max(0, Math.min(cursor, Math.max(0, approvalItems.length - 1)));
  const currentItem = approvalItems[safeCursor];
  const reviewItems = reviewItemsFromApprovals(approvalItems, approvalReviewItems);
  const reviewIndex = currentReviewIndex(reviewItems, currentItem, safeCursor);
  const progressDots = buildApprovalProgressDots(reviewItems, reviewIndex);
  const contentWidth = Math.min(columns ?? 80, 86);
  const detailWidth = Math.max(24, contentWidth - 12);
  const detailLines = currentItem ? keyValueLines(currentItem, detailWidth) : [];
  const currentRisk = currentItem ? riskForTool(currentItem) : { riskLabel: "TOOL" as const, riskColor: "gray" as const };
  const labelWidth = detailLines.reduce((max, line) => line.kind === "field" ? Math.max(max, line.key.length) : max, 0);

  return (
    <Box flexDirection="column" width={contentWidth}>
      <Text bold color="cyan">
        {formatApprovalTitle(reviewIndex, reviewItems.length || approvalItems.length)}
      </Text>
      <Box>
        <Text color="gray">Progress </Text>
        {progressDots.map((dot, index) => (
          <Text key={index} color={dot.color}>
            {dot.symbol}{index < progressDots.length - 1 ? " " : ""}
          </Text>
        ))}
      </Box>
      <Text> </Text>
      {currentItem ? (
        <Box flexDirection="column">
          <Text>
            <Text color={currentRisk.riskColor}>{currentRisk.riskLabel}</Text>
            <Text> </Text>
            <Text bold color="white">{toolLabel(currentItem)}</Text>
          </Text>
          <Text color="gray">{riskDescription(currentItem)}</Text>
          <Text> </Text>
          {detailLines.map((line, index) => {
            if (line.kind === "diff") {
              return (
                <Text key={`diff-${index}`}>
                  {line.gutter} {line.content}
                </Text>
              );
            }
            if (line.kind === "hint" || line.kind === "rule") {
              return (
                <Text key={`hint-${index}`} color="gray">{line.value}</Text>
              );
            }
            return (
              <Text key={`field-${index}-${line.key}-${line.value}`}>
                <Text color="gray">{line.key}:{ " ".repeat(Math.max(1, labelWidth - line.key.length + 1))}</Text>
                <Text color={line.color || "white"}>{line.value}</Text>
              </Text>
            );
          })}
          {detailLines.length === 0 && (
            <Text color="gray">{compactSummary(currentItem)}</Text>
          )}
        </Box>
      ) : (
        <Text color="gray">(no pending tool call)</Text>
      )}
      <Text> </Text>
      <Text color="gray">
        Y approve | N reject | A approve all | R reject all
      </Text>
    </Box>
  );
}
