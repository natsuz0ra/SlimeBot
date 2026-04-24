import assert from "node:assert/strict";
import test from "node:test";
import type { TimelineEntry } from "../types";
import { formatThinkingLabel, formatToolOutputLines, formatToolParamLines, formatPlanBlockLines } from "./Timeline";
import { stringWidth } from "../utils/stringWidth";
import { stripAnsi } from "../utils/terminal";

test("formatToolOutputLines aligns tool output with fixed spaces", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "completed",
    output: "first line",
  };

  const lines = formatToolOutputLines(entry, 120, false);

  assert.deepEqual(lines, ["   => first line"]);
});

test("formatToolOutputLines wraps long lines and indents continuation lines", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    status: "completed",
    output: "1234567890",
  };

  const lines = formatToolOutputLines(entry, 10, false);

  assert.deepEqual(lines, ["   => 1234", "      5678", "      90"]);
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

  assert.ok(lines.length >= 4);
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

  assert.equal(lines.length, 11);
  assert.ok(lines[lines.length - 1]!.includes("ctrl+o to collapse"));
});

test("formatToolOutputLines shows exec output in structured layout", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    toolName: "exec",
    command: "run",
    status: "completed",
    output: JSON.stringify({
      stdout: "hello\\nworld",
      stderr: "",
      exit_code: 0,
      timed_out: false,
      truncated: false,
      shell: "powershell",
      working_directory: "C:/repo",
      duration_ms: 20,
    }),
  };

  const lines = formatToolOutputLines(entry, 120, true);
  assert.ok(lines.some((line) => line.includes("exit_code: 0")));
  assert.ok(lines.some((line) => line.includes("stdout:")));
  assert.ok(lines.some((line) => line.includes("hello")));
});

test("formatToolParamLines pretty prints JSON params", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    params: {
      command: "echo ok",
      headers: '{"Content-Type":"application/json","x":1}',
    },
  };

  const lines = formatToolParamLines(entry, 120);

  assert.ok(lines.some((line) => line.includes("command: echo ok")));
  assert.ok(lines.some((line) => line.includes("headers:")));
  assert.ok(lines.some((line) => line.includes("Content-Type")));
});

test("formatThinkingLabel uses fixed duration after thinking completes", () => {
  const label = formatThinkingLabel({
    kind: "thinking",
    content: "",
    thinkingDone: true,
    thinkingStartedAt: 1_000,
    thinkingDurationMs: 1_750,
  });

  assert.equal(label, "Thought for 1.8s");
});

test("formatPlanBlockLines renders a closed fixed-width border", () => {
  const lines = formatPlanBlockLines("# Plan\n\nDo the thing.", 40);
  const widths = lines.map((line) => stringWidth(stripAnsi(line)));

  assert.ok(lines.length >= 4);
  assert.equal(new Set(widths).size, 1);
  assert.match(lines[0]!, /^╭.*╮$/);
  assert.match(lines[lines.length - 1]!, /^╰.*╯$/);
  assert.ok(lines.slice(1, -1).every((line) => line.startsWith("│ ") && line.endsWith(" │")));
});

test("formatPlanBlockLines keeps right border aligned for CJK content", () => {
  const lines = formatPlanBlockLines("# 更新计划\n\n检查当前版本并备份配置。", 36);
  const widths = lines.map((line) => stringWidth(stripAnsi(line)));

  assert.equal(new Set(widths).size, 1);
  assert.ok(lines.slice(1, -1).every((line) => line.endsWith(" │")));
});
