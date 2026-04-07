import assert from "node:assert/strict";
import test from "node:test";
import { renderMarkdown } from "./markdown.ts";

function stripAnsi(text: string): string {
  // eslint-disable-next-line no-control-regex
  return text.replace(/\x1b\[[0-9;]*m/g, "");
}

test("renderMarkdown formats headings/strong/list without raw markdown markers", () => {
  const input = "# Title\n\n**Bold** text\n\n- item1\n- item2";
  const output = renderMarkdown(input, 80);
  const plain = stripAnsi(output);

  assert.equal(plain.includes("# "), false);
  assert.equal(plain.includes("**"), false);
  assert.equal(plain.includes("- item1"), true);
  assert.equal(plain.includes("- item2"), true);
});

test("renderMarkdown keeps ordered list and blockquote readable", () => {
  const input = "1. first\n2. second\n\n> quote";
  const output = stripAnsi(renderMarkdown(input, 80));

  assert.equal(output.includes("1. first"), true);
  assert.equal(output.includes("2. second"), true);
  assert.equal(output.includes("│ quote"), true);
});

test("renderMarkdown keeps heading separated from paragraph", () => {
  const input = "Paragraph before heading.\n\n## Recommended";
  const output = stripAnsi(renderMarkdown(input, 80));

  assert.equal(output.includes("Recommended"), true);
  assert.equal(output.includes("Paragraph before heading."), true);
});
