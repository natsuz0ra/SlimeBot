/**
 * Terminal Markdown rendering (custom implementation).
 * Uses marked lexer tokens and recursively formats them.
 */

import chalk from "chalk";
import { marked, type Token, type Tokens } from "marked";
import { stripAnsi } from "./terminal.js";

const EOL = "\n";
let configured = false;

export function configureMarked(): void {
  if (configured) return;
  configured = true;
  marked.setOptions({
    gfm: true,
    breaks: true,
  });
}

function alignText(
  content: string,
  targetWidth: number,
  align: "left" | "center" | "right" | null | undefined,
): string {
  const visible = stripAnsi(content).length;
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

export function formatToken(
  token: Token,
  listDepth = 0,
  orderedListNumber: number | null = null,
  parent: Token | null = null,
  compact = false,
): string {
  const blockBreak = compact ? EOL : `${EOL}${EOL}`;
  switch (token.type) {
    case "heading": {
      const content = (token.tokens ?? [])
        .map((child) => formatToken(child, 0, null, token, compact))
        .join("");
      if (token.depth === 1) {
        return `${chalk.bold.underline(content)}${EOL}`;
      }
      return `${chalk.bold(content)}${EOL}`;
    }
    case "strong":
      return chalk.bold(
        (token.tokens ?? [])
          .map((child) => formatToken(child, listDepth, orderedListNumber, token, compact))
          .join(""),
      );
    case "em":
      return chalk.italic(
        (token.tokens ?? [])
          .map((child) => formatToken(child, listDepth, orderedListNumber, token, compact))
          .join(""),
      );
    case "code":
      return `${token.text}${EOL}`;
    case "codespan":
      return chalk.cyan(token.text);
    case "paragraph":
      return `${(token.tokens ?? []).map((child) => formatToken(child, 0, null, token, compact)).join("")}${compact ? "" : EOL}`;
    case "blockquote": {
      const inner = (token.tokens ?? [])
        .map((child) => formatToken(child, 0, null, token, compact))
        .join("");
      return inner
        .split(EOL)
        .map((line) => (line.trim() ? `│ ${line}` : line))
        .join(EOL);
    }
    case "list":
      return token.items
        .map((item: Token, index: number) => formatToken(
          item,
          listDepth,
          token.ordered ? (token.start || 1) + index : null,
          token,
          compact,
        ))
        .join("");
    case "list_item":
      return (token.tokens ?? [])
        .map(
          (child) => `${"  ".repeat(listDepth)}${formatToken(
            child,
            listDepth + 1,
            orderedListNumber,
            token,
            compact,
          )}`,
        )
        .join("");
    case "text":
      if (parent?.type === "list_item") {
        const marker = orderedListNumber === null ? "-" : `${orderedListNumber}.`;
        const inner = token.tokens
          ? token.tokens.map((child) => formatToken(child, listDepth, orderedListNumber, token, compact)).join("")
          : token.text;
        return `${marker} ${inner}${EOL}`;
      }
      return token.text;
    case "table": {
      const table = token as Tokens.Table;
      const widths = table.header.map((header, index) => {
        let max = stripAnsi(
          (header.tokens ?? []).map((child) => formatToken(child, 0, null, token, compact)).join(""),
        ).length;
        for (const row of table.rows) {
          const current = stripAnsi(
            (row[index]?.tokens ?? []).map((child) => formatToken(child, 0, null, token, compact)).join(""),
          ).length;
          max = Math.max(max, current);
        }
        return Math.max(3, max);
      });

      const headerLine = `| ${table.header.map((header, index) => {
        const content = (header.tokens ?? [])
          .map((child) => formatToken(child, 0, null, token, compact))
          .join("");
        return alignText(content, widths[index] || 3, table.align?.[index]);
      }).join(" | ")} |`;
      const separator = `|${widths.map((w) => "-".repeat(w + 2)).join("|")}|`;
      const rows = table.rows.map((row) => `| ${row.map((cell, index) => {
        const content = (cell.tokens ?? [])
          .map((child) => formatToken(child, 0, null, token, compact))
          .join("");
        return alignText(content, widths[index] || 3, table.align?.[index]);
      }).join(" | ")} |`).join(EOL);
      return `${headerLine}${EOL}${separator}${EOL}${rows}${EOL}${EOL}`;
    }
    case "link": {
      const textContent = (token.tokens ?? [])
        .map((child) => formatToken(child, listDepth, orderedListNumber, token, compact))
        .join("")
        .trim();
      return textContent || token.href;
    }
    case "br":
      return EOL;
    case "space":
      return compact ? "" : EOL;
    case "escape":
      return token.text;
    case "def":
    case "del":
    case "html":
      return "";
    default:
      return "";
  }
}

/** Renders Markdown text as terminal-friendly ANSI output */
export function renderMarkdown(text: string, _width = 80, compact = false): string {
  configureMarked();
  try {
    return marked
      .lexer(text)
      .map((token) => formatToken(token, 0, null, null, compact))
      .join("")
      .trim();
  } catch {
    return text;
  }
}
