import assert from "node:assert/strict";
import test from "node:test";
import type { TimelineEntry } from "../types.ts";
import { formatToolOutputLines } from "./Timeline.tsx";

test("formatToolOutputLines aligns tool output with fixed spaces", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "completed",
    output: "first line",
  };

  const lines = formatToolOutputLines(entry, 120, false);

  assert.deepEqual(lines, ["   ⎿ first line"]);
});

test("formatToolOutputLines wraps long lines and indents continuation lines", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "completed",
    output: "1234567890",
  };

  const lines = formatToolOutputLines(entry, 9, false);

  assert.deepEqual(lines, ["   ⎿ 1234", "     5678", "     90"]);
});

test("formatToolOutputLines collapses long output when not expanded", () => {
  const outputLines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`);
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "completed",
    output: outputLines.join("\n"),
  };

  const lines = formatToolOutputLines(entry, 120, false);

  // 5 preview lines + 1 hint line
  assert.ok(lines.length >= 6);
  assert.ok(lines[lines.length - 1]!.includes("ctrl+o to expand"));
});

test("formatToolOutputLines shows all lines when expanded", () => {
  const outputLines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`);
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "completed",
    output: outputLines.join("\n"),
  };

  const lines = formatToolOutputLines(entry, 120, true);

  // 10 original lines + 1 collapse hint
  assert.equal(lines.length, 11);
  assert.ok(lines[lines.length - 1]!.includes("ctrl+o to collapse"));
});

test("formatToolOutputLines shows error output in collapsed mode", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "error",
    error: Array.from({ length: 8 }, (_, i) => `err line${i + 1}`).join("\n"),
  };

  const lines = formatToolOutputLines(entry, 120, false);

  // 5 preview + 1 hint
  assert.ok(lines.length >= 6);
  assert.ok(lines[lines.length - 1]!.includes("+3 more lines"));
});
