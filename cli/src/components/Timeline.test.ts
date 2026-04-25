import assert from "node:assert/strict";
import test from "node:test";
import type { TimelineEntry } from "../types";
import {
  PLAN_GOLD,
  formatPlanningIndicatorParts,
  formatThinkingLabel,
  formatToolOutputLines,
  formatToolParamLines,
  formatPlanFrameLines,
  shouldSeparatePlanningAndWaiting,
  shouldShowWaitingPrompt,
} from "./Timeline";
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

test("PLAN_GOLD matches the frontend plan card gold", () => {
  assert.equal(PLAN_GOLD, "#f59e0b");
});

test("formatPlanningIndicatorParts uses a fixed-width blinking gold dot", () => {
  assert.deepEqual(formatPlanningIndicatorParts(true), {
    dot: "●",
    label: "Planning...",
    color: PLAN_GOLD,
  });
  assert.deepEqual(formatPlanningIndicatorParts(false), {
    dot: " ",
    label: "Planning...",
    color: PLAN_GOLD,
  });
});

test("shouldShowWaitingPrompt keeps ordinary waiting text while planning", () => {
  assert.equal(shouldShowWaitingPrompt(false, false), true);
  assert.equal(shouldShowWaitingPrompt(true, false), true);
  assert.equal(shouldShowWaitingPrompt(false, true), false);
  assert.equal(shouldShowWaitingPrompt(true, true), false);
});

test("shouldSeparatePlanningAndWaiting adds a blank line only while both prompts show", () => {
  assert.equal(shouldSeparatePlanningAndWaiting(false, false), false);
  assert.equal(shouldSeparatePlanningAndWaiting(true, false), true);
  assert.equal(shouldSeparatePlanningAndWaiting(false, true), false);
  assert.equal(shouldSeparatePlanningAndWaiting(true, true), false);
});

test("formatPlanFrameLines renders only top and bottom borders", () => {
  const lines = formatPlanFrameLines("# Plan\n\nDo the thing.", 40);
  const plain = lines.map((line) => stripAnsi(line));

  assert.ok(lines.length >= 4);
  assert.match(plain[0]!, /^─+ Plan ─+$/);
  assert.match(plain[plain.length - 1]!, /^─+$/);
  assert.equal(stringWidth(plain[0]!), 40);
  assert.equal(stringWidth(plain[plain.length - 1]!), 40);
  assert.ok(plain.slice(1, -1).every((line) => line.startsWith("  ")));
  assert.ok(plain.slice(1, -1).every((line) => !line.startsWith("│") && !line.endsWith("│")));
  assert.equal(new Set([stringWidth(plain[0]!), stringWidth(plain[plain.length - 1]!)]).size, 1);
});

test("formatPlanFrameLines keeps horizontal borders aligned for CJK content", () => {
  const lines = formatPlanFrameLines("# 更新计划\n\n检查当前版本并备份配置。", 36);
  const plain = lines.map((line) => stripAnsi(line));

  assert.equal(stringWidth(plain[0]!), stringWidth(plain[plain.length - 1]!));
  assert.ok(plain.some((line) => line.includes("更新计划")));
  assert.ok(plain.slice(1, -1).every((line) => !line.includes("│")));
});

test("formatPlanFrameLines keeps plan body spacious even when chat is compact", () => {
  const lines = formatPlanFrameLines("# Plan\n\nParagraph one.\n\n- item one\n- item two", 60);
  const plain = lines.map((line) => stripAnsi(line));
  const body = plain.slice(1, -1);

  assert.ok(body.includes("  "));
  assert.ok(body.some((line) => line.trim() === "Plan"));
  assert.ok(body.some((line) => line.trim() === "Paragraph one."));
  assert.ok(body.some((line) => line.trim() === "- item one"));
  assert.ok(body.filter((line) => line.trim() === "").length >= 2);
});
