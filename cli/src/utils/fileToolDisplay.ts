import type { TimelineEntry } from "../types.js";
import { resolve } from "node:path";

export type FileToolName = "file_read" | "file_edit" | "file_write";

export type FileReadSummary = {
  filePath: string;
  totalLines: number;
  startLine?: number;
  endLine?: number;
  truncated: boolean;
};

export type FileDiffLine = {
  kind: "context" | "added" | "removed";
  oldLine?: number;
  newLine?: number;
  text: string;
};

export type FileToolDisplay = {
  toolName: FileToolName;
  filePath: string;
  fileName: string;
  operation: "Read" | "Create" | "Update" | "Write";
  summary: string;
  diffLines: FileDiffLine[];
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : null;
}

function asArrayParam(params: Record<string, unknown> | undefined, key: string): unknown[] {
  const value = params?.[key];
  if (Array.isArray(value)) return value;
  if (typeof value === "string") {
    try {
      const parsed = JSON.parse(value);
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  }
  return [];
}

const FILE_TOOL_NAMES = new Set(["file_read", "file_edit", "file_write"]);

function baseName(filePath: string): string {
  const normalized = filePath.replace(/\\/g, "/").replace(/\/+$/, "");
  return normalized.split("/").filter(Boolean).pop() || filePath;
}

function isAbsoluteLike(filePath: string): boolean {
  return filePath.startsWith("/")
    || /^[a-zA-Z]:[\\/]/.test(filePath)
    || filePath.startsWith("\\\\");
}

function absolutePath(filePath: string): string {
  const trimmed = filePath.trim();
  if (!trimmed) return "";
  return isAbsoluteLike(trimmed) ? trimmed : resolve(trimmed);
}

function summaryForOperation(operation: FileToolDisplay["operation"], filePath: string): string {
  const absPath = absolutePath(filePath);
  if (operation === "Create") return `Created ${absPath}`;
  if (operation === "Update") return `Updated ${absPath}`;
  if (operation === "Write") return `Wrote ${absPath}`;
  return `Read ${absPath}`;
}

export function isFileToolName(toolName: string | undefined): toolName is FileToolName {
  return FILE_TOOL_NAMES.has((toolName || "").trim().toLowerCase());
}

export function isFileToolEntry(entry: TimelineEntry): boolean {
  return isFileToolName(entry.toolName);
}

export function countTextLines(content: string): number {
  if (content === "") return 0;
  const parts = content.split("\n");
  return content.endsWith("\n") ? parts.length - 1 : parts.length;
}

function splitTextLines(content: string): string[] {
  if (content === "") return [];
  const lines = content.replace(/\r\n/g, "\n").split("\n");
  if (content.endsWith("\n")) lines.pop();
  return lines;
}

export function parseFileReadOutput(raw: string): FileReadSummary | null {
  const filePath = raw.match(/^File:\s*(.+)$/m)?.[1]?.trim();
  const totalRaw = raw.match(/^Total lines:\s*(\d+)$/m)?.[1];
  if (!filePath || totalRaw === undefined) return null;

  const showing = raw.match(/^Showing lines\s+(\d+)-(\d+):$/m) || raw.match(/^Range\s+\d+\s+lines\s+(\d+)-(\d+):$/m);
  return {
    filePath,
    totalLines: Number(totalRaw),
    startLine: showing ? Number(showing[1]) : undefined,
    endLine: showing ? Number(showing[2]) : undefined,
    truncated: raw.includes("[truncated;"),
  };
}

export function formatFileReadSummary(summary: FileReadSummary): string {
  if (
    summary.startLine !== undefined &&
    summary.endLine !== undefined &&
    (summary.startLine !== 1 || summary.endLine !== summary.totalLines)
  ) {
    const readLines = Math.max(0, summary.endLine - summary.startLine + 1);
    return `Read ${readLines} of ${summary.totalLines} lines, showing ${summary.startLine}-${summary.endLine}`;
  }
  return `Read ${summary.totalLines} ${summary.totalLines === 1 ? "line" : "lines"}`;
}

export function buildLineDiff(oldContent: string, newContent: string): FileDiffLine[] {
  const oldLines = splitTextLines(oldContent);
  const newLines = splitTextLines(newContent);
  const rows = oldLines.length + 1;
  const cols = newLines.length + 1;
  const table: number[][] = Array.from({ length: rows }, () => Array.from({ length: cols }, () => 0));

  for (let i = oldLines.length - 1; i >= 0; i--) {
    for (let j = newLines.length - 1; j >= 0; j--) {
      table[i]![j] = oldLines[i] === newLines[j]
        ? table[i + 1]![j + 1]! + 1
        : Math.max(table[i + 1]![j]!, table[i]![j + 1]!);
    }
  }

  const result: FileDiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < oldLines.length && j < newLines.length) {
    if (oldLines[i] === newLines[j]) {
      result.push({ kind: "context", oldLine: i + 1, newLine: j + 1, text: oldLines[i]! });
      i++;
      j++;
    } else if (table[i + 1]![j]! >= table[i]![j + 1]!) {
      result.push({ kind: "removed", oldLine: i + 1, text: oldLines[i]! });
      i++;
    } else {
      result.push({ kind: "added", newLine: j + 1, text: newLines[j]! });
      j++;
    }
  }
  while (i < oldLines.length) {
    result.push({ kind: "removed", oldLine: i + 1, text: oldLines[i]! });
    i++;
  }
  while (j < newLines.length) {
    result.push({ kind: "added", newLine: j + 1, text: newLines[j]! });
    j++;
  }
  return result;
}

function metadataDiffLine(value: unknown): FileDiffLine | null {
  if (!value || typeof value !== "object") return null;
  const raw = value as Record<string, unknown>;
  const kind = raw.kind;
  if (kind !== "context" && kind !== "added" && kind !== "removed") return null;
  const oldLine = typeof raw.oldLine === "number" && Number.isFinite(raw.oldLine) ? raw.oldLine : undefined;
  const newLine = typeof raw.newLine === "number" && Number.isFinite(raw.newLine) ? raw.newLine : undefined;
  const line: FileDiffLine = {
    kind,
    text: typeof raw.text === "string" ? raw.text : "",
  };
  if (oldLine !== undefined) line.oldLine = oldLine;
  if (newLine !== undefined) line.newLine = newLine;
  return line;
}

function displayFromMetadata(toolName: FileToolName, metadata: unknown): FileToolDisplay | null {
  if (!metadata || typeof metadata !== "object") return null;
  const raw = metadata as Record<string, unknown>;
  const operation = raw.operation;
  if (operation !== "Read" && operation !== "Create" && operation !== "Update" && operation !== "Write") return null;
  const filePath = typeof raw.filePath === "string" ? raw.filePath.trim() : "";
  if (!filePath) return null;
  const diffLines = Array.isArray(raw.diffLines)
    ? raw.diffLines.map(metadataDiffLine).filter((line): line is FileDiffLine => !!line)
    : [];
  return {
    toolName,
    filePath,
    fileName: baseName(filePath),
    operation,
    summary: operation === "Read" && typeof raw.summary === "string" && raw.summary.trim()
      ? raw.summary.trim()
      : summaryForOperation(operation, filePath),
    diffLines,
  };
}

function displaysFromMetadata(toolName: FileToolName, metadata: unknown): FileToolDisplay[] {
  if (Array.isArray(metadata)) {
    return metadata.map((item) => displayFromMetadata(toolName, item)).filter((item): item is FileToolDisplay => !!item);
  }
  const single = displayFromMetadata(toolName, metadata);
  return single ? [single] : [];
}

function buildBatchSummary(toolName: FileToolName, params: Record<string, unknown>): string {
  if (toolName === "file_read") {
    const requests = asArrayParam(params, "requests");
    return requests.length > 0 ? `Read ${requests.length} ${requests.length === 1 ? "file" : "files"}` : "Read file";
  }
  if (toolName === "file_write") {
    const writes = asArrayParam(params, "writes");
    return writes.length > 0 ? `Write ${writes.length} ${writes.length === 1 ? "file" : "files"}` : "Write file";
  }
  const edits = asArrayParam(params, "edits");
  if (edits.length === 0) return "Update file";
  let creates = 0;
  let updates = 0;
  for (const item of edits) {
    const record = asRecord(item);
    const operations = Array.isArray(record?.operations) ? record.operations : [];
    const first = asRecord(operations[0]);
    const oldString = String(first?.old_string ?? "");
    if (oldString === "") creates++;
    else updates++;
  }
  const parts: string[] = [];
  if (updates > 0) parts.push(`Update ${updates} ${updates === 1 ? "file" : "files"}`);
  if (creates > 0) parts.push(`Create ${creates} ${creates === 1 ? "file" : "files"}`);
  return parts.length > 0 ? parts.join(" / ") : `Edit ${edits.length} ${edits.length === 1 ? "file" : "files"}`;
}

export function buildFileToolDisplays(entry: {
  toolName?: string;
  params?: Record<string, unknown>;
  output?: string;
  error?: string;
  content?: string;
  metadata?: unknown;
}): FileToolDisplay[] {
  const toolName = (entry.toolName || "").trim().toLowerCase();
  if (!isFileToolName(toolName)) return [];

  const metadataDisplays = displaysFromMetadata(toolName, entry.metadata);
  if (metadataDisplays.length > 0) return metadataDisplays;

  const params = entry.params || {};
  const filePath = String(params.file_path || "").trim();
  if (toolName === "file_read") {
    const parsed = parseFileReadOutput(entry.output || entry.content || "");
    const path = parsed?.filePath || filePath;
    const summary = parsed
      ? formatFileReadSummary(parsed)
      : filePath
        ? `Read ${baseName(filePath)}`
        : buildBatchSummary(toolName, params);
    return [{
      toolName,
      filePath: path,
      fileName: baseName(path),
      operation: "Read",
      summary,
      diffLines: [],
    }];
  }

  if (toolName === "file_edit") {
    const oldString = String(params.old_string ?? "");
    const newString = String(params.new_string ?? "");
    const creating = oldString === "";
    return [{
      toolName,
      filePath,
      fileName: baseName(filePath),
      operation: creating ? "Create" : "Update",
      summary: filePath ? summaryForOperation(creating ? "Create" : "Update", filePath) : buildBatchSummary(toolName, params),
      diffLines: filePath ? buildLineDiff(oldString, newString) : [],
    }];
  }

  const content = String(params.content ?? "");
  const lineCount = countTextLines(content);
  return [{
    toolName,
    filePath,
    fileName: baseName(filePath),
    operation: "Write",
    summary: filePath ? `Wrote ${lineCount} ${lineCount === 1 ? "line" : "lines"} to ${absolutePath(filePath)}` : buildBatchSummary(toolName, params),
    diffLines: filePath ? buildLineDiff("", content) : [],
  }];
}

export function buildFileToolDisplay(entry: {
  toolName?: string;
  params?: Record<string, unknown>;
  output?: string;
  error?: string;
  content?: string;
  metadata?: unknown;
}): FileToolDisplay | null {
  return buildFileToolDisplays(entry)[0] || null;
}

export function fileToolSummaryFromParams(toolName: string, params?: Record<string, unknown>): string {
  const name = toolName.trim().toLowerCase();
  if (!isFileToolName(name)) return "";
  const normalized = params || {};
  const filePath = String(normalized.file_path ?? "").trim();
  if (!filePath) {
    if (name === "file_read") {
      const requests = asArrayParam(normalized, "requests");
      return requests.length > 0 ? `Read ${requests.length} ${requests.length === 1 ? "file" : "files"}` : "";
    }
    if (name === "file_write") {
      const writes = asArrayParam(normalized, "writes");
      return writes.length > 0 ? `Write ${writes.length} ${writes.length === 1 ? "file" : "files"}` : "";
    }
    const edits = asArrayParam(normalized, "edits");
    if (edits.length > 0) {
      let creates = 0;
      let updates = 0;
      for (const item of edits) {
        const record = asRecord(item);
        const operations = Array.isArray(record?.operations) ? record.operations : [];
        const first = asRecord(operations[0]);
        if (String(first?.old_string ?? "") === "") creates++;
        else updates++;
      }
      const parts: string[] = [];
      if (updates > 0) parts.push(`Update ${updates} ${updates === 1 ? "file" : "files"}`);
      if (creates > 0) parts.push(`Create ${creates} ${creates === 1 ? "file" : "files"}`);
      return parts.join(" / ");
    }
    return "";
  }
  if (name === "file_read") return `Read ${baseName(filePath)}`;
  if (name === "file_edit") return `${String(normalized.old_string ?? "") === "" ? "Create" : "Update"} ${baseName(filePath)}`;
  return `Write ${baseName(filePath)}`;
}
