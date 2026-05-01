import React from "react";
import { Box, Text } from "ink";
import type { TimelineEntry } from "../types.js";
import type { FileDiffLine } from "../utils/fileToolDisplay.js";
import { buildFileToolDisplays } from "../utils/fileToolDisplay.js";
import { renderColorDiffRows } from "../native/colorDiff.js";

const MAX_PREVIEW_LINES = 8;
const BORDER_COLOR = "#64748b";
const SUMMARY_COLOR = "#cbd5e1";
const HINT_COLOR = "#94a3b8";

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

  const width = Math.max(24, Math.min(maxWidth - 3, maxWidth));
  return (
    <Box marginLeft={3} flexDirection="column">
      {displays.map((display, idx) => {
        const previewLines = expanded ? display.diffLines : display.diffLines.slice(0, MAX_PREVIEW_LINES);
        const remaining = display.diffLines.length - previewLines.length;
        const renderedRows = chunkDiffLines(previewLines).flatMap((chunk, index) => {
          const rows = renderColorDiffRows({
            filePath: display.filePath,
            lines: chunk,
            width: Math.max(12, width - 2),
          });
          return index === 0 ? rows : [{ gutter: "", content: "..." }, ...rows];
        });
        return (
          <Box key={`${display.filePath}-${display.operation}-${idx}`} flexDirection="column">
            <Text color={SUMMARY_COLOR}>└─ {display.summary}</Text>
            {display.toolName === "file_read" || display.diffLines.length === 0 ? null : (
              <Box flexDirection="column" borderStyle="single" borderColor={BORDER_COLOR} borderLeft={false} borderRight={false}>
                <Box flexDirection="row">
                  <Box flexDirection="column" flexShrink={0}>
                    {renderedRows.map((row, index) => (
                      <Text key={`gutter-${idx}-${index}`}>{row.gutter}</Text>
                    ))}
                  </Box>
                  <Box flexDirection="column" marginLeft={1}>
                    {renderedRows.map((row, index) => (
                      <Text key={`content-${idx}-${index}`}>{row.content}</Text>
                    ))}
                  </Box>
                </Box>
                {remaining > 0 ? (
                  <Text color={HINT_COLOR}>... +{remaining} more changed lines (ctrl+o to expand)</Text>
                ) : expanded && display.diffLines.length > MAX_PREVIEW_LINES ? (
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
