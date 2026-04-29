import assert from "node:assert/strict";
import test from "node:test";
import type { TimelineEntry } from "../types";
import {
  PLAN_GOLD,
  WAITING_STATS_COLOR,
  formatSubagentThinkingLines,
  formatRunSubagentDetailLines,
  formatPlanningIndicatorParts,
  formatThinkingLabel,
  formatToolOutputLines,
  formatToolParamLines,
  formatPlanFrameLines,
  formatWaitingPromptText,
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

test("formatSubagentThinkingLines shows live child reasoning while sub-agent is still running", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    toolName: "run_subagent",
    status: "executing",
    subagentThinking: {
      content: "checking files before answering",
      thinkingDone: false,
    },
  };

  const lines = formatSubagentThinkingLines(entry, 80, false);

  assert.equal(lines[0], "   Sub-agent thinking...");
  assert.ok(lines.some((line) => line.includes("checking files before answering")));
});

test("formatSubagentThinkingLines collapses completed child reasoning by default", () => {
  const entry: TimelineEntry = {
    kind: "tool",
    content: "",
    toolName: "run_subagent",
    status: "completed",
    subagentThinking: {
      content: Array.from({ length: 8 }, (_, i) => `child reason ${i + 1}`).join("\n"),
      thinkingDone: true,
      thinkingDurationMs: 1200,
    },
  };

  const lines = formatSubagentThinkingLines(entry, 80, false);

  assert.equal(lines[0], "   Sub-agent thought for 1.2s");
  assert.ok(lines.some((line) => line.includes("ctrl+o to expand")));
});

function runSubagentFixture(): { parent: TimelineEntry; nested: TimelineEntry[] } {
  return {
    parent: {
      kind: "tool",
      content: "",
      toolCallId: "parent",
      toolName: "run_subagent",
      command: "delegate",
      status: "completed",
      params: {
        context: "repo context line 1\nrepo context line 2\nrepo context line 3\nrepo context line 4",
        task: "inspect the display ordering\nthen report a concise answer",
        priority: "high",
      },
      output: "final result line 1\nfinal result line 2\nfinal result line 3\nfinal result line 4",
      subagentThinking: {
        content: "child reason 1\nchild reason 2\nchild reason 3\nchild reason 4",
        thinkingDone: true,
        thinkingDurationMs: 1250,
      },
    },
    nested: [{
      kind: "tool",
      content: "hits",
      toolCallId: "child",
      toolName: "web_search",
      command: "search",
      status: "completed",
      output: "hits",
      parentToolCallId: "parent",
      subagentRunId: "run-1",
    }],
  };
}

test("formatRunSubagentDetailLines follows frontend section order", () => {
  const { parent, nested } = runSubagentFixture();
  const lines = formatRunSubagentDetailLines(parent, nested, 100, false);

  const contextIndex = lines.findIndex((line) => line.includes("Context"));
  const taskIndex = lines.findIndex((line) => line.includes("Task"));
  const thinkingIndex = lines.findIndex((line) => line.includes("Thinking & tools"));
  const resultIndex = lines.findIndex((line) => line.includes("Result"));

  assert.ok(contextIndex >= 0);
  assert.ok(contextIndex < taskIndex);
  assert.ok(taskIndex < thinkingIndex);
  assert.ok(thinkingIndex < resultIndex);
  assert.ok(lines.some((line) => line.includes("├─ Context →")));
  assert.ok(lines.some((line) => line.includes("└─ Result →")));
});

test("formatRunSubagentDetailLines keeps collapsed display to one-line summaries", () => {
  const { parent, nested } = runSubagentFixture();
  const lines = formatRunSubagentDetailLines(parent, nested, 100, false);

  assert.ok(lines.some((line) => line.includes("Context → repo context line 1 ... +3 more lines")));
  assert.ok(lines.some((line) => line.includes("Task → inspect the display ordering ... +1 more lines")));
  assert.ok(lines.some((line) => line.includes("Thinking & tools: thinking complete in 1.3s · 1 tool")));
  assert.ok(lines.some((line) => line.includes("Result → final result line 1")));
  assert.ok(lines.every((line) => !line.includes("child reason 4")));
  assert.ok(lines.every((line) => !line.includes("final result line 4")));
});

test("formatRunSubagentDetailLines aligns wrapped collapsed summaries under the value", () => {
  const { parent, nested } = runSubagentFixture();
  parent.output = "This is a very long subagent result summary that must wrap onto a continuation line";

  const lines = formatRunSubagentDetailLines(parent, nested, 40, false);
  const resultIndex = lines.findIndex((line) => line.includes("Result →"));

  assert.ok(resultIndex >= 0);
  const resultLine = lines[resultIndex]!;
  const continuationLine = lines[resultIndex + 1]!;
  const valueStart = resultLine.indexOf("This");
  const continuationIndent = continuationLine.match(/^ */)?.[0].length ?? 0;

  assert.ok(valueStart > 0);
  assert.ok(continuationIndent > 0);
  assert.equal(continuationIndent, valueStart);
});

test("formatRunSubagentDetailLines expands full details and keeps collapse hint", () => {
  const { parent, nested } = runSubagentFixture();
  const lines = formatRunSubagentDetailLines(parent, nested, 100, true);

  assert.ok(lines.some((line) => line.includes("repo context line 4")));
  assert.ok(lines.some((line) => line.includes("child reason 4")));
  assert.ok(lines.some((line) => line.includes("final result line 4")));
  assert.ok(lines.some((line) => line.includes("ctrl+o to collapse")));
});

test("formatRunSubagentDetailLines hides context and task from extra params", () => {
  const { parent, nested } = runSubagentFixture();
  const lines = formatRunSubagentDetailLines(parent, nested, 100, false);
  const paramsIndex = lines.findIndex((line) => line.includes("Params"));
  const afterParams = paramsIndex >= 0 ? lines.slice(paramsIndex) : [];

  assert.ok(afterParams.some((line) => line.includes("priority: high")));
  assert.ok(afterParams.every((line) => !line.includes("context:")));
  assert.ok(afterParams.every((line) => !line.includes("task:")));
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

test("formatWaitingPromptText appends suffix when provided", () => {
  assert.equal(
    formatWaitingPromptText("(13m 27s · ↑ 23.7k tokens)"),
    " Waiting for response... (13m 27s · ↑ 23.7k tokens)",
  );
});

test("formatWaitingPromptText keeps original prompt without suffix", () => {
  assert.equal(formatWaitingPromptText(""), " Waiting for response...");
});

test("WAITING_STATS_COLOR matches the chat footer hint color", () => {
  assert.equal(WAITING_STATS_COLOR, "#64748b");
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
