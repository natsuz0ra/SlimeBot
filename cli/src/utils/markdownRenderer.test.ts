import assert from "node:assert/strict";
import test from "node:test";
import { stripAnsi } from "./terminal";
import { renderMarkdownLines } from "./markdownRenderer";

test("renderMarkdownLines renders markdown tables with box borders", () => {
  const input = "| Name | Role |\n| ---- | ---- |\n| Alice | Dev |";
  const lines = renderMarkdownLines(input, 80, false);
  const joined = stripAnsi(lines.join("\n"));

  assert.equal(joined.includes("┌"), true);
  assert.equal(joined.includes("│"), true);
  assert.equal(joined.includes("└"), true);
  assert.equal(joined.includes("| Name |"), false);
});

test("renderMarkdownLines compact mode reduces blank lines", () => {
  const input = "# Title\n\nParagraph one.\n\nParagraph two.";
  const normal = renderMarkdownLines(input, 80, false).filter((line) => line.trim() === "").length;
  const compact = renderMarkdownLines(input, 80, true).filter((line) => line.trim() === "").length;

  assert.equal(compact < normal, true);
});

test("renderMarkdownLines compact mode keeps heading on its own line after paragraph", () => {
  const input = "Paragraph before heading.\n\n## Recommended\n\n- item1";
  const lines = renderMarkdownLines(input, 80, true).map((l) => stripAnsi(l));

  assert.equal(lines.some((l) => l === "Paragraph before heading."), true);
  assert.equal(lines.some((l) => l === "Recommended"), true);
  assert.equal(lines.some((l) => l.includes("Paragraph before heading.") && l.includes("Recommended")), false);
});

test("renderMarkdownLines compact mode no blank line between list and heading", () => {
  const input = "## Heading1\n\n- item1\n- item2\n\n## Heading2\n\n- item3";
  const lines = renderMarkdownLines(input, 80, true).map((l) => stripAnsi(l));
  const blankCount = lines.filter((l) => l.trim() === "").length;

  assert.equal(blankCount, 0, "compact mode should have no blank lines");
});

test("renderMarkdownLines non-compact mode preserves plan section spacing", () => {
  const input = "# Plan\n\nParagraph one.\n\n- item1\n- item2\n\n## Verify\n\nRun tests.";
  const normal = renderMarkdownLines(input, 80, false).map((l) => stripAnsi(l));
  const compact = renderMarkdownLines(input, 80, true).map((l) => stripAnsi(l));

  assert.ok(normal.filter((line) => line.trim() === "").length >= 3);
  assert.equal(compact.filter((line) => line.trim() === "").length, 0);
});

test("renderMarkdownLines keeps wrapped list continuation indented", () => {
  const input = "- this item has enough detail to wrap onto another line without merging into the next paragraph";
  const lines = renderMarkdownLines(input, 36, true).map((l) => stripAnsi(l));

  assert.ok(lines.length > 1);
  assert.equal(lines[0]!.startsWith("- "), true);
  assert.ok(lines.slice(1).every((line) => line.startsWith("  ")));
});
