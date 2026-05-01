import { createRequire } from "node:module";
import { basename, extname } from "node:path";
import { diffWordsWithSpace } from "diff";
import hljs from "highlight.js";
import type { FileDiffLine } from "../utils/fileToolDisplay.js";
import { stringWidth } from "../utils/stringWidth.js";
import { stripAnsi } from "../utils/terminal.js";

export type ColorDiffLine = Pick<FileDiffLine, "kind" | "oldLine" | "newLine" | "text">;

export type RenderColorDiffInput = {
  filePath: string;
  lines: ColorDiffLine[];
  width: number;
};

export type RenderedColorDiffLine = {
  gutter: string;
  content: string;
};

type NativeColorDiffModule = {
  renderColorDiffJson?: (input: string) => string;
  render_color_diff_json?: (input: string) => string;
};

const require = createRequire(import.meta.url);

let nativeLoadAttempted = false;
let nativeModule: NativeColorDiffModule | null = null;

const RESET = "\x1b[0m";
const FG_DIM = "\x1b[38;5;244m";
const FG_ADD = "\x1b[38;5;114m";
const FG_DEL = "\x1b[38;5;203m";
const FG_CONTEXT = "\x1b[38;5;250m";
const BG_ADD = "\x1b[48;5;22m";
const BG_DEL = "\x1b[48;5;52m";
const BG_ADD_WORD = "\x1b[48;5;28m";
const BG_DEL_WORD = "\x1b[48;5;88m";

const SCOPE_COLORS: Record<string, string> = {
  keyword: "\x1b[38;5;204m",
  built_in: "\x1b[38;5;149m",
  type: "\x1b[38;5;149m",
  literal: "\x1b[38;5;141m",
  number: "\x1b[38;5;141m",
  string: "\x1b[38;5;186m",
  title: "\x1b[38;5;149m",
  "title.function": "\x1b[38;5;149m",
  "title.class": "\x1b[38;5;149m",
  params: "\x1b[38;5;215m",
  comment: "\x1b[38;5;102m",
  meta: "\x1b[38;5;102m",
  attr: "\x1b[38;5;149m",
  attribute: "\x1b[38;5;149m",
  variable: "\x1b[38;5;231m",
  property: "\x1b[38;5;231m",
  operator: "\x1b[38;5;204m",
  punctuation: "\x1b[38;5;252m",
  symbol: "\x1b[38;5;141m",
  regexp: "\x1b[38;5;186m",
};

export function loadNativeColorDiff(): NativeColorDiffModule | null {
  if (nativeLoadAttempted) return nativeModule;
  nativeLoadAttempted = true;
  try {
    nativeModule = require("@slimebot/color-diff-native") as NativeColorDiffModule;
  } catch {
    nativeModule = null;
  }
  return nativeModule;
}

export function setNativeColorDiffForTest(module: NativeColorDiffModule | null): void {
  nativeLoadAttempted = true;
  nativeModule = module;
}

export function renderColorDiff(input: RenderColorDiffInput): string[] {
  return renderColorDiffRows(input).map((line) => `${line.gutter} ${line.content}`);
}

export function renderColorDiffRows(input: RenderColorDiffInput): RenderedColorDiffLine[] {
  const native = loadNativeColorDiff();
  const renderJson = native?.renderColorDiffJson ?? native?.render_color_diff_json;
  if (renderJson) {
    try {
      const parsed = JSON.parse(renderJson(JSON.stringify(input)));
      if (isRenderedRows(parsed)) return parsed;
    } catch {
      // Fall through to the TypeScript renderer.
    }
  }
  return renderColorDiffRowsFallback(input);
}

function renderColorDiffRowsFallback(input: RenderColorDiffInput): RenderedColorDiffLine[] {
  const language = languageForFile(input.filePath);
  const gutterWidth = computeGutterWidth(input.lines);
  const contentWidth = Math.max(8, input.width - gutterWidth - 1);
  const pairedChanges = wordChangeMap(input.lines);

  return input.lines.map((line, index) => {
    const marker = markerForKind(line.kind);
    const lineNo = line.kind === "added" ? line.newLine : line.oldLine ?? line.newLine;
    const gutterText = `${marker} ${lineNo === undefined ? "" : String(lineNo).padStart(gutterWidth - 2, " ")}`;
    const markerColor = line.kind === "added" ? FG_ADD : line.kind === "removed" ? FG_DEL : FG_DIM;
    const bg = line.kind === "added" ? BG_ADD : line.kind === "removed" ? BG_DEL : "";
    const content = highlightLine(line.text || " ", language, line.kind);
    const changedRanges = pairedChanges.get(index);
    const emphasized = changedRanges ? applyWordBackground(content, changedRanges, line.kind) : content;
    return {
      gutter: `${markerColor}${gutterText}${RESET}`,
      content: `${bg}${truncateAnsi(emphasized, contentWidth)}${RESET}`,
    };
  });
}

function isRenderedRows(value: unknown): value is RenderedColorDiffLine[] {
  return Array.isArray(value) && value.every((item) => (
    item &&
    typeof item === "object" &&
    typeof (item as RenderedColorDiffLine).gutter === "string" &&
    typeof (item as RenderedColorDiffLine).content === "string"
  ));
}

function markerForKind(kind: ColorDiffLine["kind"]): string {
  if (kind === "added") return "+";
  if (kind === "removed") return "-";
  return " ";
}

function computeGutterWidth(lines: ColorDiffLine[]): number {
  let maxLine = 1;
  for (const line of lines) {
    maxLine = Math.max(maxLine, line.oldLine ?? 0, line.newLine ?? 0);
  }
  return String(maxLine).length + 2;
}

function languageForFile(filePath: string): string | undefined {
  const base = basename(filePath).toLowerCase();
  const ext = extname(base).replace(/^\./, "");
  if (base === "go.mod" || base === "go.sum") return "go";
  if (base === "package.json" || base.endsWith(".json")) return "json";
  const aliases: Record<string, string> = {
    ts: "typescript",
    tsx: "typescript",
    js: "javascript",
    jsx: "javascript",
    vue: "xml",
    go: "go",
    rs: "rust",
    py: "python",
    sh: "bash",
    zsh: "bash",
    md: "markdown",
    css: "css",
    html: "xml",
    xml: "xml",
    yaml: "yaml",
    yml: "yaml",
  };
  return aliases[ext];
}

function highlightLine(text: string, language: string | undefined, kind: ColorDiffLine["kind"]): string {
  const fallbackColor = kind === "added" ? FG_ADD : kind === "removed" ? FG_DEL : FG_CONTEXT;
  if (!language || !hljs.getLanguage(language)) return `${fallbackColor}${escapeAnsiText(text)}`;

  try {
    const html = hljs.highlight(text, { language, ignoreIllegals: true }).value;
    const highlighted = htmlToAnsi(html, fallbackColor);
    return highlighted || `${fallbackColor}${escapeAnsiText(text)}`;
  } catch {
    return `${fallbackColor}${escapeAnsiText(text)}`;
  }
}

function htmlToAnsi(html: string, fallbackColor: string): string {
  let out = fallbackColor;
  const stack: string[] = [fallbackColor];
  const tokenPattern = /<span class="([^"]+)">|<\/span>|&(?:amp|lt|gt|quot|#39);|[^<&]+/g;
  for (const match of html.matchAll(tokenPattern)) {
    const [token, className] = match;
    if (className) {
      const color = colorForClass(className) || stack[stack.length - 1] || fallbackColor;
      stack.push(color);
      out += color;
      continue;
    }
    if (token === "</span>") {
      stack.pop();
      out += stack[stack.length - 1] || fallbackColor;
      continue;
    }
    out += decodeHtmlEntity(token);
  }
  return out;
}

function colorForClass(className: string): string | undefined {
  const scopes = className
    .split(/\s+/)
    .map((item) => item.replace(/^hljs-/, ""))
    .filter(Boolean);
  for (let i = scopes.length; i > 0; i--) {
    const key = scopes.slice(0, i).join(".");
    if (SCOPE_COLORS[key]) return SCOPE_COLORS[key];
  }
  return scopes.map((scope) => SCOPE_COLORS[scope]).find(Boolean);
}

function decodeHtmlEntity(token: string): string {
  return token
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, "\"")
    .replace(/&#39;/g, "'");
}

function escapeAnsiText(text: string): string {
  return text.replace(/\x1b/g, "");
}

function wordChangeMap(lines: ColorDiffLine[]): Map<number, Array<[number, number]>> {
  const ranges = new Map<number, Array<[number, number]>>();
  for (let i = 0; i < lines.length - 1; i++) {
    const current = lines[i]!;
    const next = lines[i + 1]!;
    if (current.kind !== "removed" || next.kind !== "added") continue;
    const diff = diffWordsWithSpace(current.text, next.text);
    let oldPos = 0;
    let newPos = 0;
    const oldRanges: Array<[number, number]> = [];
    const newRanges: Array<[number, number]> = [];
    for (const part of diff) {
      const len = part.value.length;
      if (part.removed) {
        oldRanges.push([oldPos, oldPos + len]);
        oldPos += len;
      } else if (part.added) {
        newRanges.push([newPos, newPos + len]);
        newPos += len;
      } else {
        oldPos += len;
        newPos += len;
      }
    }
    if (oldRanges.length > 0) ranges.set(i, oldRanges);
    if (newRanges.length > 0) ranges.set(i + 1, newRanges);
  }
  return ranges;
}

function applyWordBackground(ansiText: string, ranges: Array<[number, number]>, kind: ColorDiffLine["kind"]): string {
  const plain = stripAnsi(ansiText);
  if (plain.length === 0) return ansiText;
  const bg = kind === "removed" ? BG_DEL_WORD : BG_ADD_WORD;
  let visible = 0;
  let out = "";
  let active = false;
  for (let i = 0; i < ansiText.length; i++) {
    if (ansiText[i] === "\x1b") {
      const end = ansiText.indexOf("m", i);
      if (end >= 0) {
        out += ansiText.slice(i, end + 1);
        if (active) out += bg;
        i = end;
        continue;
      }
    }
    const shouldBeActive = ranges.some(([start, end]) => visible >= start && visible < end);
    if (shouldBeActive !== active) {
      out += shouldBeActive ? bg : RESET;
      active = shouldBeActive;
    }
    out += ansiText[i];
    visible++;
  }
  if (active) out += RESET;
  return out;
}

function truncateAnsi(input: string, maxWidth: number): string {
  if (maxWidth <= 0) return "";
  const ellipsis = "…";
  const ellipsisWidth = stringWidth(ellipsis);
  const plain = stripAnsi(input);
  if (stringWidth(plain) <= maxWidth) return input;
  const limit = Math.max(0, maxWidth - ellipsisWidth);
  let visible = 0;
  let out = "";
  for (let i = 0; i < input.length; i++) {
    if (input[i] === "\x1b") {
      const end = input.indexOf("m", i);
      if (end >= 0) {
        out += input.slice(i, end + 1);
        i = end;
        continue;
      }
    }
    const cp = input.codePointAt(i);
    if (cp === undefined) break;
    const char = String.fromCodePoint(cp);
    const width = stringWidth(char);
    if (visible + width > limit) break;
    out += char;
    visible += width;
    if (cp > 0xffff) i++;
  }
  return `${out}${ellipsis}`;
}
