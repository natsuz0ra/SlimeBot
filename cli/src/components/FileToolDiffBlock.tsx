import React from "react";
import { Box, Text } from "ink";
import type { TimelineEntry } from "../types.js";
import type { FileDiffLine } from "../utils/fileToolDisplay.js";
import { buildFileToolDisplays } from "../utils/fileToolDisplay.js";
import { renderColorDiffRows } from "../native/colorDiff.js";

const MAX_PREVIEW_LINES = 8;
const MAX_FILE_EDIT_RENDER_LINES = 3000;
const BORDER_COLOR = "#475569";
const FILE_EDIT_BORDER_COLOR = "#ffffff";
const SUMMARY_COLOR = "#cbd5e1";
const HINT_COLOR = "#94a3b8";
const FILE_EDIT_SEPARATOR_COLOR = "#9ca3af";

interface FileToolDiffBlockProps {
  entry: TimelineEntry;
  maxWidth: number;
  expanded: boolean;
}

function lineNumber(line: FileDiffLine): number | undefined {
  return line.newLine ?? line.oldLine;
}

function chunkDiffLines(lines: FileDiffLine[]): FileDiffLine[][] {
  const chunks: FileDiffLine[][] = [];
  let current: FileDiffLine[] = [];
  let previousLine: number | undefined;
  for (const line of lines) {
    const currentLine = lineNumber(line);
    if (current.length > 0 && previousLine !== undefined && currentLine !== undefined && currentLine - previousLine > 1) {
      chunks.push(current);
      current = [];
    }
    current.push(line);
    if (currentLine !== undefined) previousLine = currentLine;
  }
  if (current.length > 0) chunks.push(current);
  return chunks;
}

export function FileToolDiffBlock({
  entry,
  maxWidth,
  expanded,
}: FileToolDiffBlockProps): React.ReactElement | null {
  const displays = buildFileToolDisplays(entry);
  if (displays.length === 0) return null;

  if (entry.status === "error" || entry.status === "rejected") {
    return (
      <Box marginLeft={3} flexDirection="column">
        <Text color="red">{entry.error || entry.content || "File tool failed"}</Text>
      </Box>
    );
  }

  if (entry.status !== "completed") return null;

  const blockHorizontalInset = 3;
  const width = Math.max(24, maxWidth - (blockHorizontalInset * 2));
  return (
    <Box marginLeft={blockHorizontalInset} marginRight={blockHorizontalInset} flexDirection="column">
      {displays.map((display, idx) => {
        const isFileEdit = display.toolName === "file_edit";
        const hardLimited = isFileEdit && display.diffLines.length > MAX_FILE_EDIT_RENDER_LINES;
        const effectiveLines = hardLimited
          ? display.diffLines.slice(0, MAX_FILE_EDIT_RENDER_LINES)
          : display.diffLines;
        const previewLines = isFileEdit
          ? effectiveLines
          : (expanded ? effectiveLines : effectiveLines.slice(0, MAX_PREVIEW_LINES));
        const remaining = effectiveLines.length - previewLines.length;
        const renderedRows = chunkDiffLines(previewLines).flatMap((chunk, index) => {
          const rows = renderColorDiffRows({
            filePath: display.filePath,
            lines: chunk,
            width: Math.max(12, width - 2),
          });
          return index === 0 ? rows : [{ gutter: "", content: "─".repeat(Math.max(8, width - 2)) }, ...rows];
        });
        return (
          <Box key={`${display.filePath}-${display.operation}-${idx}`} flexDirection="column">
            <Text color={SUMMARY_COLOR}>└─ {display.summary}</Text>
            {display.toolName === "file_read" || display.diffLines.length === 0 ? null : (
              <Box
                flexDirection="column"
                borderStyle="single"
                borderColor={isFileEdit ? FILE_EDIT_BORDER_COLOR : BORDER_COLOR}
                borderLeft={false}
                borderRight={false}
              >
                {renderedRows.map((row, index) => (
                  <Text
                    key={`row-${idx}-${index}`}
                    color={row.gutter === "" ? FILE_EDIT_SEPARATOR_COLOR : undefined}
                  >
                    {row.gutter === "" ? row.content : `${row.gutter} ${row.content}`}
                  </Text>
                ))}
                {hardLimited ? (
                  <Text color={HINT_COLOR}>output safety limit reached: showing first {MAX_FILE_EDIT_RENDER_LINES} changed lines</Text>
                ) : remaining > 0 ? (
                  <Text color={HINT_COLOR}>... +{remaining} more changed lines (ctrl+o to expand)</Text>
                ) : expanded && !isFileEdit && display.diffLines.length > MAX_PREVIEW_LINES ? (
                  <Text color={HINT_COLOR}>... (ctrl+o to collapse)</Text>
                ) : null}
              </Box>
            )}
          </Box>
        );
      })}
    </Box>
  );
}
