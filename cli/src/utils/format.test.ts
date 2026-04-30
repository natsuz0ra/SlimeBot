import assert from "node:assert/strict";
import test from "node:test";
import {
  wrapText,
  formatCollapsedLines,
  formatToolTextValue,
  formatToolParamEntries,
  filterToolParamsForDetail,
  formatToolCallSummary,
  parseExecOutputPayload,
  estimateTokens,
  formatCompactTokenCount,
  formatTurnDuration,
  formatWaitingStatsSuffix,
} from "./format";

test("wrapText wraps plain text with target width", () => {
  const wrapped = wrapText("1234567890", 4);
  assert.equal(wrapped, "1234\n5678\n90");
});

test("formatCollapsedLines returns all lines for short text", () => {
  const result = formatCollapsedLines("line1\nline2\nline3", 5, false);
  assert.deepEqual(result.lines, ["line1", "line2", "line3"]);
  assert.equal(result.totalLines, 3);
});

test("formatCollapsedLines collapses long text when not expanded", () => {
  const lines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`);
  const text = lines.join("\n");
  const result = formatCollapsedLines(text, 5, false);
  assert.equal(result.totalLines, 10);
  assert.equal(result.lines.length, 6);
  assert.ok(result.lines[5]!.includes("+5 more lines"));
  assert.ok(result.lines[5]!.includes("ctrl+o to expand"));
});

test("formatCollapsedLines shows all lines with hint when expanded", () => {
  const lines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`);
  const text = lines.join("\n");
  const result = formatCollapsedLines(text, 5, true);
  assert.equal(result.totalLines, 10);
  assert.equal(result.lines.length, 11);
  assert.ok(result.lines[10]!.includes("ctrl+o to collapse"));
});

test("formatCollapsedLines handles empty text", () => {
  const result = formatCollapsedLines("", 5, false);
  assert.deepEqual(result.lines, ["(No output)"]);
  assert.equal(result.totalLines, 1);
});

test("formatToolTextValue pretty prints JSON object", () => {
  const result = formatToolTextValue('{"a":1,"b":{"c":2}}');
  assert.ok(result.includes("\n"));
  assert.ok(result.includes('"a": 1'));
});

test("formatToolTextValue decodes escaped newlines", () => {
  const result = formatToolTextValue("line1\\nline2");
  assert.equal(result, "line1\nline2");
});

test("formatToolTextValue trims leading and trailing blank lines", () => {
  const result = formatToolTextValue("\n\nline1\\n\\nline2\n\n");
  assert.equal(result, "line1\nline2");
});

test("formatToolParamEntries formats multiline JSON values", () => {
  const entries = formatToolParamEntries({
    a: "plain",
    b: '{"x":1,"y":2}',
  });
  assert.ok(entries[0]!.startsWith("a:"));
  assert.ok(entries.some((line) => line.includes("b:")));
  assert.ok(entries.some((line) => line.includes('"x": 1')));
});

test("formatToolCallSummary uses core tool parameters", () => {
  assert.equal(
    formatToolCallSummary("exec", "run", { command: "go test ./...", description: "Run Go tests" }),
    "Run Go tests",
  );
  assert.equal(
    formatToolCallSummary("web_search", "search", { query: "SlimeBot latest" }),
    "query: SlimeBot latest",
  );
  assert.equal(
    formatToolCallSummary("http_request", "request", { method: "post", url: "https://example.test/api" }),
    "POST https://example.test/api",
  );
  assert.equal(
    formatToolCallSummary("run_subagent", "delegate", {
      title: "Inspect UI cards",
      task: "Inspect UI cards and report exact files",
    }),
    "Inspect UI cards",
  );
  assert.equal(
    formatToolCallSummary("run_subagent", "delegate", { task: "Inspect UI cards and report exact files" }),
    "task: Inspect UI cards and report exact files",
  );
});

test("formatToolCallSummary hides missing legacy exec description", () => {
  assert.equal(formatToolCallSummary("exec", "run", { command: "go test ./..." }), "");
});

test("filterToolParamsForDetail removes params already shown in summary", () => {
  assert.deepEqual(
    filterToolParamsForDetail("exec", "run", { command: "go test ./...", description: "Run Go tests" }),
    { command: "go test ./..." },
  );
  assert.deepEqual(
    filterToolParamsForDetail("web_search", "search", { query: "SlimeBot latest" }),
    {},
  );
  assert.deepEqual(
    filterToolParamsForDetail("run_subagent", "delegate", {
      title: "Inspect UI cards",
      task: "Inspect UI cards and report exact files",
      context: "repo state",
      priority: "high",
    }),
    { context: "repo state", priority: "high" },
  );
});

test("parseExecOutputPayload parses valid exec output payload", () => {
  const payload = parseExecOutputPayload(JSON.stringify({
    stdout: "ok",
    stderr: "",
    exit_code: 0,
    timed_out: false,
    truncated: false,
    shell: "powershell",
    working_directory: "C:/repo",
    duration_ms: 12,
  }));
  assert.ok(payload);
  assert.equal(payload?.exit_code, 0);
});

test("parseExecOutputPayload returns null on invalid payload", () => {
  const payload = parseExecOutputPayload('{"stdout":"ok"}');
  assert.equal(payload, null);
});

test("formatTurnDuration formats seconds, minutes, and hours", () => {
  assert.equal(formatTurnDuration(5_200), "5s");
  assert.equal(formatTurnDuration(13 * 60_000 + 27_000), "13m 27s");
  assert.equal(formatTurnDuration(64 * 60_000 + 12_000), "1h 04m");
});

test("formatCompactTokenCount formats plain, thousand, and million counts", () => {
  assert.equal(formatCompactTokenCount(987), "987 tokens");
  assert.equal(formatCompactTokenCount(23_700), "23.7k tokens");
  assert.equal(formatCompactTokenCount(1_240_000), "1.2m tokens");
});

test("estimateTokens is deterministic and nonzero for normal text", () => {
  const text = "Update the store with streamed thinking and tool output.";

  assert.equal(estimateTokens(text), estimateTokens(text));
  assert.ok(estimateTokens(text) > 0);
});

test("formatWaitingStatsSuffix omits thought duration when unavailable", () => {
  assert.equal(
    formatWaitingStatsSuffix({
      elapsedMs: 13 * 60_000 + 27_000,
      tokenEstimate: 23_700,
    }),
    "(13m 27s · ↑ 23.7k tokens)",
  );
});

test("formatWaitingStatsSuffix includes thought duration when present", () => {
  assert.equal(
    formatWaitingStatsSuffix({
      elapsedMs: 13 * 60_000 + 27_000,
      tokenEstimate: 23_700,
      thoughtDurationMs: 5_000,
    }),
    "(13m 27s · ↑ 23.7k tokens · thought for 5s)",
  );
});

test("formatWaitingStatsSuffix shows active thinking before duration is available", () => {
  assert.equal(
    formatWaitingStatsSuffix({
      elapsedMs: 13 * 60_000 + 27_000,
      tokenEstimate: 23_700,
      thinkingActive: true,
    }),
    "(13m 27s · ↑ 23.7k tokens · thinking)",
  );
});

test("formatWaitingStatsSuffix prefers active thinking over thought duration", () => {
  assert.equal(
    formatWaitingStatsSuffix({
      elapsedMs: 13 * 60_000 + 27_000,
      tokenEstimate: 23_700,
      thoughtDurationMs: 5_000,
      thinkingActive: true,
    }),
    "(13m 27s · ↑ 23.7k tokens · thinking)",
  );
});
