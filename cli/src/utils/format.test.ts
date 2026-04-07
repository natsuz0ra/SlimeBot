import assert from "node:assert/strict";
import test from "node:test";
import { wrapText, formatCollapsedLines, TOOL_OUTPUT_PREVIEW_LINES } from "./format.ts";

test("wrapText wraps plain text with target width", () => {
  const wrapped = wrapText("1234567890", 4);
  assert.equal(wrapped, "1234\n5678\n90");
});

test("wrapText handles CJK width correctly", () => {
  const wrapped = wrapText("这是一个很长的中文文本", 6);
  assert.equal(wrapped, "这是一\n个很长\n的中文\n文本");
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
  assert.equal(result.lines.length, 6); // 5 preview + 1 hint
  assert.ok(result.lines[5]!.includes("+5 more lines"));
  assert.ok(result.lines[5]!.includes("ctrl+o to expand"));
});

test("formatCollapsedLines shows all lines with hint when expanded", () => {
  const lines = Array.from({ length: 10 }, (_, i) => `line${i + 1}`);
  const text = lines.join("\n");
  const result = formatCollapsedLines(text, 5, true);
  assert.equal(result.totalLines, 10);
  assert.equal(result.lines.length, 11); // 10 original + 1 hint
  assert.ok(result.lines[10]!.includes("ctrl+o to collapse"));
});

test("formatCollapsedLines handles empty text", () => {
  const result = formatCollapsedLines("", 5, false);
  assert.deepEqual(result.lines, ["(No output)"]);
  assert.equal(result.totalLines, 1);
});

test("formatCollapsedLines short text has no hint regardless of expanded state", () => {
  const result = formatCollapsedLines("line1\nline2", 5, true);
  assert.deepEqual(result.lines, ["line1", "line2"]);
  assert.equal(result.totalLines, 2);
});
