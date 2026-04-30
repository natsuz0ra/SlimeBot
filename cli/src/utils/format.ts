import wrapAnsi from "wrap-ansi";

/** Formats tool invocation text shown in timeline rows. */
export function formatToolInvocation(toolName: string, command: string): string {
  const name = toolName.trim() || "tool";
  const cmd = command.trim() || "run";
  return `${name}.${cmd}()`;
}

function normalizedParam(params: Record<string, string> | undefined, key: string): string {
  return String(params?.[key] ?? "").trim();
}

function firstNonEmptyParam(params?: Record<string, string>): { key: string; value: string } | null {
  if (!params) return null;
  for (const key of Object.keys(params).sort()) {
    const value = normalizedParam(params, key);
    if (value !== "") return { key, value };
  }
  return null;
}

export function getToolSummaryParamKeys(
  toolName: string,
  command: string,
  params?: Record<string, string>,
): string[] {
  const tool = toolName.trim().toLowerCase();
  const cmd = command.trim().toLowerCase();
  if (tool === "") return [];

  if (tool === "exec" && cmd === "run" && normalizedParam(params, "description") !== "") {
    return ["description"];
  }
  if ((tool === "web_search" || tool === "search_memory") && normalizedParam(params, "query") !== "") {
    return ["query"];
  }
  if (tool === "run_subagent") {
    const keys: string[] = [];
    if (normalizedParam(params, "title") !== "") keys.push("title");
    if (normalizedParam(params, "task") !== "") keys.push("task");
    if (keys.length > 0) return keys;
  }
  if (tool === "http_request" && cmd === "request") {
    const keys: string[] = [];
    if (normalizedParam(params, "method") !== "") keys.push("method");
    if (normalizedParam(params, "url") !== "") keys.push("url");
    if (keys.length > 0) return keys;
  }

  const fallback = firstNonEmptyParam(params);
  return fallback ? [fallback.key] : [];
}

export function formatToolCallSummary(
  toolName: string,
  command: string,
  params?: Record<string, string>,
): string {
  const tool = toolName.trim().toLowerCase();
  const cmd = command.trim().toLowerCase();

  if (tool === "exec" && cmd === "run") {
    return normalizedParam(params, "description");
  }
  if (tool === "web_search" || tool === "search_memory") {
    const query = normalizedParam(params, "query");
    return query ? `query: ${query}` : "";
  }
  if (tool === "run_subagent") {
    const title = normalizedParam(params, "title");
    if (title) return title;
    const task = normalizedParam(params, "task");
    return task ? `task: ${task}` : "";
  }
  if (tool === "http_request" && cmd === "request") {
    const method = normalizedParam(params, "method").toUpperCase();
    const url = normalizedParam(params, "url");
    return [method, url].filter(Boolean).join(" ");
  }

  const fallback = firstNonEmptyParam(params);
  return fallback ? `${fallback.key}: ${fallback.value}` : "";
}

export function filterToolParamsForDetail(
  toolName: string,
  command: string,
  params?: Record<string, string>,
): Record<string, string> {
  if (!params) return {};
  const hidden = new Set(getToolSummaryParamKeys(toolName, command, params));
  const result: Record<string, string> = {};
  for (const [key, value] of Object.entries(params)) {
    if (!hidden.has(key)) result[key] = value;
  }
  return result;
}

type JSONValue = null | boolean | number | string | JSONValue[] | { [k: string]: JSONValue };

export interface ExecOutputPayload {
  stdout: string;
  stderr: string;
  exit_code: number;
  timed_out: boolean;
  truncated: boolean;
  shell: string;
  working_directory: string;
  duration_ms: number;
}

function isJSONObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function tryParseJSON(raw: string): unknown | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;
  try {
    return JSON.parse(trimmed);
  } catch {
    return null;
  }
}

function decodeCommonEscapes(raw: string): string {
  if (!raw.includes("\\")) return raw;
  return raw
    .replace(/\\r\\n/g, "\n")
    .replace(/\\n/g, "\n")
    .replace(/\\r/g, "\n")
    .replace(/\\t/g, "\t")
    .replace(/\\\\"/g, '"')
    .replace(/\\\\/g, "\\");
}

/** Formats one tool text value, attempting JSON pretty-print and common escape decoding. */
export function formatToolTextValue(raw: string): string {
  const parsed = tryParseJSON(raw);
  if (parsed !== null) {
    if (typeof parsed === "string") {
      return decodeCommonEscapes(parsed);
    }
    try {
      return JSON.stringify(parsed as JSONValue, null, 2);
    } catch {
      return raw;
    }
  }
  const decoded = decodeCommonEscapes(raw);
  // Filter consecutive empty lines in display only
  return decoded.replace(/\n{2,}/g, "\n").trim();
}

/** Formats params into readable key/value lines. */
export function formatToolParamEntries(params?: Record<string, string>): string[] {
  if (!params || Object.keys(params).length === 0) return [];
  const keys = Object.keys(params).sort();
  const lines: string[] = [];
  for (const key of keys) {
    const value = formatToolTextValue(params[key] ?? "");
    const segments = value.split(/\r?\n/);
    if (segments.length <= 1) {
      lines.push(`${key}: ${segments[0]}`);
      continue;
    }
    lines.push(`${key}:`);
    for (const seg of segments) {
      lines.push(`  ${seg}`);
    }
  }
  return lines;
}

export function parseExecOutputPayload(raw: string): ExecOutputPayload | null {
  const parsed = tryParseJSON(raw);
  if (!isJSONObject(parsed)) return null;

  const stdout = parsed.stdout;
  const stderr = parsed.stderr;
  const exitCode = parsed.exit_code;
  const timedOut = parsed.timed_out;
  const truncated = parsed.truncated;
  const shell = parsed.shell;
  const workingDirectory = parsed.working_directory;
  const durationMs = parsed.duration_ms;

  if (
    typeof stdout !== "string" ||
    typeof stderr !== "string" ||
    typeof exitCode !== "number" ||
    typeof timedOut !== "boolean" ||
    typeof truncated !== "boolean" ||
    typeof shell !== "string" ||
    typeof workingDirectory !== "string" ||
    typeof durationMs !== "number"
  ) {
    return null;
  }

  return {
    stdout,
    stderr,
    exit_code: exitCode,
    timed_out: timedOut,
    truncated,
    shell,
    working_directory: workingDirectory,
    duration_ms: durationMs,
  };
}

export function formatTurnDuration(durationMs: number): string {
  const totalSeconds = Math.max(0, Math.floor(durationMs / 1000));
  if (totalSeconds < 60) {
    return `${totalSeconds}s`;
  }
  const totalMinutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (totalMinutes < 60) {
    return `${totalMinutes}m ${seconds}s`;
  }
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return `${hours}h ${String(minutes).padStart(2, "0")}m`;
}

export function formatCompactTokenCount(tokens: number): string {
  const count = Math.max(0, Math.round(tokens));
  if (count < 1_000) {
    return `${count} tokens`;
  }
  if (count < 1_000_000) {
    return `${(count / 1_000).toFixed(1)}k tokens`;
  }
  return `${(count / 1_000_000).toFixed(1)}m tokens`;
}

export function estimateTokens(text: string): number {
  const normalized = (text ?? "").trim();
  if (!normalized) return 0;
  const cjkChars = normalized.match(/[\u3400-\u9fff\uf900-\ufaff]/g)?.length ?? 0;
  const wordLike = normalized.match(/[A-Za-z0-9_]+|[^\sA-Za-z0-9_\u3400-\u9fff\uf900-\ufaff]/g)?.length ?? 0;
  const charEstimate = Math.ceil(normalized.length / 4);
  return Math.max(1, Math.ceil(Math.max(charEstimate, cjkChars + wordLike)));
}

export function formatWaitingStatsSuffix(stats: {
  elapsedMs: number;
  tokenEstimate: number;
  thoughtDurationMs?: number;
  thinkingActive?: boolean;
}): string {
  const parts = [
    formatTurnDuration(stats.elapsedMs),
    `↑ ${formatCompactTokenCount(stats.tokenEstimate)}`,
  ];
  if (stats.thinkingActive) {
    parts.push("thinking");
  } else if (stats.thoughtDurationMs !== undefined) {
    parts.push(`thought for ${formatTurnDuration(stats.thoughtDurationMs)}`);
  }
  return `(${parts.join(" · ")})`;
}

/** Formats tool output for display. Exec output gets a structured layout when possible. */
export function formatToolExecutionOutput(toolName: string, command: string, raw: string): string {
  const normalizedTool = toolName.trim().toLowerCase();
  const normalizedCommand = command.trim().toLowerCase();
  if (normalizedTool === "exec" && normalizedCommand === "run") {
    const payload = parseExecOutputPayload(raw);
    if (payload) {
      const lines: string[] = [
        `exit_code: ${payload.exit_code} | timed_out: ${payload.timed_out} | truncated: ${payload.truncated} | duration_ms: ${payload.duration_ms} | shell: ${payload.shell}`,
      ];

      if (payload.stdout.trim()) {
        lines.push("stdout:");
        lines.push(formatToolTextValue(payload.stdout));
      }
      if (payload.stderr.trim()) {
        lines.push("stderr:");
        lines.push(formatToolTextValue(payload.stderr));
      }
      if (!payload.stdout.trim() && !payload.stderr.trim()) {
        lines.push("(No output)");
      }
      return lines.join("\n");
    }
  }
  return formatToolTextValue(raw);
}

/** Truncates multi-line text into a single-line preview. */
export function truncateText(text: string, maxLen: number): string {
  const singleLine = text.replace(/\r?\n/g, " ").replace(/\s+/g, " ").trim();
  if (!singleLine) return "(No output)";
  if (singleLine.length <= maxLen) return singleLine;
  const suffix = "...[truncated]";
  return singleLine.slice(0, maxLen - suffix.length) + suffix;
}

/** Default number of preview lines shown when tool output is collapsed. */
export const TOOL_OUTPUT_PREVIEW_LINES = 3;

/**
 * Formats tool output lines with collapsible support.
 * Returns the lines to display and the total line count.
 * - Short output (<= maxPreviewLines): all lines, no hint.
 * - Collapsed: first maxPreviewLines lines + expand hint.
 * - Expanded: all lines + collapse hint.
 */
export function formatCollapsedLines(
  text: string,
  maxPreviewLines: number,
  expanded: boolean,
): { lines: string[]; totalLines: number } {
  const normalized = (text ?? "").replace(/\r\n/g, "\n").trim();
  if (!normalized) {
    return { lines: ["(No output)"], totalLines: 1 };
  }

  const rawLines = normalized.split("\n");
  const totalLines = rawLines.length;

  if (totalLines <= maxPreviewLines) {
    return { lines: rawLines, totalLines };
  }

  if (expanded) {
    return {
      lines: [...rawLines, "... (ctrl+o to collapse)"],
      totalLines,
    };
  }

  const preview = rawLines.slice(0, maxPreviewLines);
  const remaining = totalLines - maxPreviewLines;
  preview.push(`... +${remaining} more lines (ctrl+o to expand)`);
  return { lines: preview, totalLines };
}

/** Pre-wraps text for terminal width, preserving ANSI and CJK width. */
export function wrapText(text: string, maxWidth: number): string {
  const normalized = (text ?? "").replace(/\r\n/g, "\n");
  const width = Math.max(1, Math.floor(maxWidth));
  return wrapAnsi(normalized, width, {
    hard: true,
    trim: false,
  });
}

/** Format ISO timestamp into local readable date-time string. */
export function formatTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

/** Parsed ask_questions readable answer item. */
export interface AskQuestionsReadableAnswer {
  id: string;
  question: string;
  answer: string;
}

/**
 * Parses the output of a completed ask_questions tool call.
 * Backend formats output as: [{"id":"...","question":"...","answer":"..."}]
 * Returns null if the output cannot be parsed.
 */
export function parseAskQuestionsReadableAnswers(raw: string): AskQuestionsReadableAnswer[] | null {
  const parsed = tryParseJSON(raw);
  if (!Array.isArray(parsed)) return null;
  const answers: AskQuestionsReadableAnswer[] = [];
  for (const item of parsed) {
    if (typeof item !== "object" || item === null || Array.isArray(item)) return null;
    const { id, question, answer } = item as Record<string, unknown>;
    if (typeof id !== "string" || typeof question !== "string" || typeof answer !== "string") return null;
    answers.push({ id, question, answer });
  }
  return answers.length > 0 ? answers : null;
}

/**
 * Formats ask_questions output into human-readable tree lines for the CLI timeline.
 * Uses ├─/└─ tree connectors with "question → answer" per line.
 * Returns null if parsing fails (caller should fall back to raw output).
 */
export function formatAskQuestionsDisplay(raw: string): string[] | null {
  const answers = parseAskQuestionsReadableAnswers(raw);
  if (!answers) return null;
  const lines: string[] = [];
  for (let i = 0; i < answers.length; i++) {
    const a = answers[i];
    const connector = i < answers.length - 1 ? "├─" : "└─";
    const answer = a.answer || "(Not selected)";
    lines.push(`${connector} ${a.question} → ${answer}`);
  }
  return lines;
}

/**
 * Formats ask_questions pending state into tree lines showing questions and their options.
 * Input is the raw JSON from params["questions"].
 * Returns null if parsing fails.
 */
export function formatAskQuestionsPending(raw: string): string[] | null {
  const parsed = tryParseJSON(raw);
  if (!Array.isArray(parsed) || parsed.length === 0) return null;
  const lines: string[] = [];
  for (let i = 0; i < parsed.length; i++) {
    const item = parsed[i];
    if (typeof item !== "object" || item === null) return null;
    const { question, options } = item as Record<string, unknown>;
    if (typeof question !== "string" || !Array.isArray(options)) return null;
    const connector = i < parsed.length - 1 ? "├─" : "└─";
    lines.push(`${connector} ${question}`);
    for (let j = 0; j < options.length; j++) {
      const isLastOption = j === options.length - 1;
      const optConnector = isLastOption ? "└─" : "├─";
      const indent = i < parsed.length - 1 ? "│  " : "   ";
      lines.push(`${indent}${optConnector} ${options[j]}`);
    }
  }
  return lines;
}
