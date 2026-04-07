import assert from "node:assert/strict";
import test from "node:test";
import {
  formatMenuDescriptionLines,
  truncateMenuDescription,
  truncateMenuTitle,
} from "./MenuView.tsx";

test("truncateMenuTitle truncates long titles with ellipsis", () => {
  const input = "12345678901234567890123456";
  assert.equal(truncateMenuTitle(input), "123456789012345678901234…");
});

test("truncateMenuDescription limits description to 80 characters", () => {
  const input = "a".repeat(100);
  const output = truncateMenuDescription(input);

  assert.equal(output.length, 80);
  assert.ok(output.endsWith("…"));
});

test("formatMenuDescriptionLines wraps by terminal width", () => {
  const lines = formatMenuDescriptionLines(
    "Use when user asks to run a Python script locally to write files.",
    24,
  );

  assert.ok(lines.length > 1);
  assert.ok(lines.every((line) => line.length <= 22));
});
