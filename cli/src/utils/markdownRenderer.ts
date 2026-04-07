import { marked, type Token, type Tokens } from "marked";
import { wrapText } from "./format.js";
import { configureMarked, formatToken } from "./markdown.js";
import { stringWidth } from "./stringWidth.js";
import { stripAnsi } from "./terminal.js";
import { wrapAnsi } from "./wrapAnsi.js";

const SAFETY_MARGIN = 4;
const MIN_COLUMN_WIDTH = 3;
const MAX_ROW_LINES = 4;
const ANSI_BOLD_START = "\x1b[1m";
const ANSI_BOLD_END = "\x1b[22m";

type Align = "left" | "center" | "right" | null | undefined;

function wrapCellText(text: string, width: number, hard = false): string[] {
  if (width <= 0) return [text];
  const wrapped = wrapAnsi(text.trimEnd(), width, {
    hard,
    trim: false,
    wordWrap: true,
  });
  const lines = wrapped.split("\n").filter((line) => line.length > 0);
  return lines.length > 0 ? lines : [""];
}

function padAligned(content: string, targetWidth: number, align: Align): string {
  const visible = stringWidth(stripAnsi(content));
  const padding = Math.max(0, targetWidth - visible);
  if (align === "center") {
    const left = Math.floor(padding / 2);
    return `${" ".repeat(left)}${content}${" ".repeat(padding - left)}`;
  }
  if (align === "right") {
    return `${" ".repeat(padding)}${content}`;
  }
  return `${content}${" ".repeat(padding)}`;
}

function formatCellTokens(tokens: Token[] | undefined, compact: boolean): string {
  return tokens?.map((child) => formatToken(child, 0, null, null, compact)).join("") ?? "";
}

function getDisplayText(tokens: Token[] | undefined, compact: boolean): string {
  return stripAnsi(formatCellTokens(tokens, compact));
}

function getLongestWordWidth(tokens: Token[] | undefined, compact: boolean): number {
  const text = getDisplayText(tokens, compact);
  const words = text.split(/\s+/).filter((word) => word.length > 0);
  if (words.length === 0) return MIN_COLUMN_WIDTH;
  return Math.max(...words.map((word) => stringWidth(word)), MIN_COLUMN_WIDTH);
}

function getIdealWidth(tokens: Token[] | undefined, compact: boolean): number {
  return Math.max(stringWidth(getDisplayText(tokens, compact)), MIN_COLUMN_WIDTH);
}

export function renderTableLines(
  token: Tokens.Table,
  maxWidth: number,
  compact = false,
): string[] {
  const terminalWidth = Math.max(1, Math.floor(maxWidth));
  const numCols = token.header.length;
  if (numCols === 0) return [];

  const minWidths = token.header.map((header, colIndex) => {
    let maxMinWidth = getLongestWordWidth(header.tokens, compact);
    for (const row of token.rows) {
      maxMinWidth = Math.max(maxMinWidth, getLongestWordWidth(row[colIndex]?.tokens, compact));
    }
    return maxMinWidth;
  });

  const idealWidths = token.header.map((header, colIndex) => {
    let maxIdeal = getIdealWidth(header.tokens, compact);
    for (const row of token.rows) {
      maxIdeal = Math.max(maxIdeal, getIdealWidth(row[colIndex]?.tokens, compact));
    }
    return maxIdeal;
  });

  const borderOverhead = 1 + numCols * 3;
  const availableWidth = Math.max(
    terminalWidth - borderOverhead - SAFETY_MARGIN,
    numCols * MIN_COLUMN_WIDTH,
  );
  const totalMin = minWidths.reduce((sum, width) => sum + width, 0);
  const totalIdeal = idealWidths.reduce((sum, width) => sum + width, 0);

  let needsHardWrap = false;
  let columnWidths: number[];

  if (totalIdeal <= availableWidth) {
    columnWidths = idealWidths;
  } else if (totalMin <= availableWidth) {
    const extraSpace = availableWidth - totalMin;
    const overflows = idealWidths.map((ideal, i) => ideal - minWidths[i]!);
    const totalOverflow = overflows.reduce((sum, overflow) => sum + overflow, 0);
    columnWidths = minWidths.map((min, i) => {
      if (totalOverflow === 0) return min;
      const extra = Math.floor((overflows[i]! / totalOverflow) * extraSpace);
      return min + extra;
    });
  } else {
    needsHardWrap = true;
    const scaleFactor = availableWidth / totalMin;
    columnWidths = minWidths.map((width) =>
      Math.max(Math.floor(width * scaleFactor), MIN_COLUMN_WIDTH)
    );
  }

  function calculateMaxRowLines(): number {
    let maxLines = 1;
    for (let i = 0; i < token.header.length; i++) {
      const content = formatCellTokens(token.header[i]!.tokens, compact);
      maxLines = Math.max(maxLines, wrapCellText(content, columnWidths[i]!, needsHardWrap).length);
    }
    for (const row of token.rows) {
      for (let i = 0; i < row.length; i++) {
        const content = formatCellTokens(row[i]?.tokens, compact);
        maxLines = Math.max(maxLines, wrapCellText(content, columnWidths[i]!, needsHardWrap).length);
      }
    }
    return maxLines;
  }

  function renderRowLines(
    cells: Array<{
      tokens?: Token[];
    }>,
    isHeader: boolean,
  ): string[] {
    const cellLines = cells.map((cell, colIndex) =>
      wrapCellText(
        formatCellTokens(cell.tokens, compact),
        columnWidths[colIndex]!,
        needsHardWrap,
      )
    );
    const maxLines = Math.max(1, ...cellLines.map((lines) => lines.length));
    const verticalOffsets = cellLines.map((lines) => Math.floor((maxLines - lines.length) / 2));

    const result: string[] = [];
    for (let lineIndex = 0; lineIndex < maxLines; lineIndex++) {
      let line = "│";
      for (let colIndex = 0; colIndex < cells.length; colIndex++) {
        const lines = cellLines[colIndex]!;
        const offset = verticalOffsets[colIndex]!;
        const contentLineIndex = lineIndex - offset;
        const rawText =
          contentLineIndex >= 0 && contentLineIndex < lines.length ? lines[contentLineIndex]! : "";
        const text = isHeader ? `${ANSI_BOLD_START}${rawText}${ANSI_BOLD_END}` : rawText;
        const width = columnWidths[colIndex]!;
        const align = isHeader ? "center" : (token.align?.[colIndex] ?? "left");
        line += ` ${padAligned(text, width, align)} │`;
      }
      result.push(line);
    }
    return result;
  }

  function renderBorderLine(type: "top" | "middle" | "bottom"): string {
    const [left, mid, cross, right] = {
      top: ["┌", "─", "┬", "┐"],
      middle: ["├", "─", "┼", "┤"],
      bottom: ["└", "─", "┴", "┘"],
    }[type] as [string, string, string, string];
    let line = left;
    columnWidths.forEach((width, index) => {
      line += mid.repeat(width + 2);
      line += index < columnWidths.length - 1 ? cross : right;
    });
    return line;
  }

  function renderVerticalFormatLines(): string[] {
    const lines: string[] = [];
    const headers = token.header.map((header) => getDisplayText(header.tokens, compact));
    const separator = "─".repeat(Math.max(1, Math.min(terminalWidth - 1, 40)));
    const wrapIndent = "  ";

    token.rows.forEach((row, rowIndex) => {
      if (rowIndex > 0) lines.push(separator);
      row.forEach((cell, colIndex) => {
        const label = headers[colIndex] || `Column ${colIndex + 1}`;
        const rawValue = formatCellTokens(cell.tokens, compact).trimEnd();
        const value = rawValue.replace(/\n+/g, " ").replace(/\s+/g, " ").trim();

        const firstLineWidth = Math.max(10, terminalWidth - stringWidth(label) - 3);
        const continuationWidth = Math.max(10, terminalWidth - wrapIndent.length - 1);
        const firstPass = wrapCellText(value, firstLineWidth, true);
        const firstLine = firstPass[0] || "";
        let wrapped = firstPass;
        if (firstPass.length > 1 && continuationWidth > firstLineWidth) {
          const remainingText = firstPass.slice(1).map((line) => line.trim()).join(" ");
          wrapped = [firstLine, ...wrapCellText(remainingText, continuationWidth, true)];
        }

        lines.push(`${ANSI_BOLD_START}${label}:${ANSI_BOLD_END} ${wrapped[0] || ""}`);
        for (let i = 1; i < wrapped.length; i++) {
          const line = wrapped[i]!;
          if (!line.trim()) continue;
          lines.push(`${wrapIndent}${line}`);
        }
      });
    });
    return lines;
  }

  if (calculateMaxRowLines() > MAX_ROW_LINES) {
    return renderVerticalFormatLines();
  }

  const lines: string[] = [];
  lines.push(renderBorderLine("top"));
  lines.push(...renderRowLines(token.header, true));
  lines.push(renderBorderLine("middle"));
  token.rows.forEach((row, rowIndex) => {
    lines.push(...renderRowLines(row, false));
    if (rowIndex < token.rows.length - 1) {
      lines.push(renderBorderLine("middle"));
    }
  });
  lines.push(renderBorderLine("bottom"));

  const maxLineWidth = Math.max(...lines.map((line) => stringWidth(stripAnsi(line))));
  if (maxLineWidth > terminalWidth - SAFETY_MARGIN) {
    return renderVerticalFormatLines();
  }
  return lines;
}

export function renderMarkdownLines(
  content: string,
  maxWidth: number,
  compact = false,
): string[] {
  configureMarked();

  const terminalWidth = Math.max(1, Math.floor(maxWidth));
  const tokens = marked.lexer(content ?? "");
  const lines: string[] = [];
  let buffer = "";

  function flushBuffer(): void {
    if (!buffer) return;
    const wrapped = wrapText(buffer.trimEnd(), terminalWidth);
    lines.push(...wrapped.split("\n"));
    buffer = "";
  }

  const BLOCK_TYPES = new Set(["heading", "table", "list", "blockquote", "code"]);

  for (const token of tokens) {
    if (BLOCK_TYPES.has(token.type)) {
      flushBuffer();
    }
    if (token.type === "table") {
      lines.push(...renderTableLines(token as Tokens.Table, terminalWidth, compact));
      continue;
    }
    buffer += formatToken(token, 0, null, null, compact);
  }

  flushBuffer();

  while (lines.length > 1 && lines[lines.length - 1] === "") {
    lines.pop();
  }
  return lines.length > 0 ? lines : [""];
}
