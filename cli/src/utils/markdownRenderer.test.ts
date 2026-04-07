import assert from "node:assert/strict";
import test from "node:test";
import { stripAnsi } from "./terminal.ts";
import { renderMarkdownLines } from "./markdownRenderer.ts";

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
