import assert from "node:assert/strict";
import test from "node:test";
import {
  renderColorDiff,
  renderColorDiffRows,
  setNativeColorDiffForTest,
} from "./colorDiff";
import { stringWidth } from "../utils/stringWidth";
import { stripAnsi } from "../utils/terminal";

test("renderColorDiffRows uses native renderer when available", () => {
  setNativeColorDiffForTest({
    renderColorDiffJson: () => JSON.stringify([{ gutter: "N 1", content: "native" }]),
  });

  const rows = renderColorDiffRows({
    filePath: "src/example.ts",
    width: 80,
    lines: [{ kind: "added", newLine: 1, text: "const ok = true" }],
  });

  assert.deepEqual(rows, [{ gutter: "N 1", content: "native" }]);
  setNativeColorDiffForTest(null);
});

test("renderColorDiffRows falls back to TypeScript renderer when native fails", () => {
  setNativeColorDiffForTest({
    renderColorDiffJson: () => {
      throw new Error("native unavailable");
    },
  });

  const rows = renderColorDiffRows({
    filePath: "src/example.go",
    width: 80,
    lines: [{ kind: "added", newLine: 1, text: "func main() {}" }],
  });

  assert.equal(stripAnsi(rows[0]!.gutter).trim(), "+ 1");
  assert.ok(stripAnsi(rows[0]!.content).includes("func main()"));
  assert.match(rows[0]!.content, /\x1b\[/);
  setNativeColorDiffForTest(null);
});

test("renderColorDiff returns ANSI lines with plain content intact", () => {
  setNativeColorDiffForTest(null);
  const lines = renderColorDiff({
    filePath: "src/example.ts",
    width: 80,
    lines: [
      { kind: "removed", oldLine: 1, text: "const oldName = 1" },
      { kind: "added", newLine: 1, text: "const newName = 1" },
    ],
  });
  const plain = lines.map((line) => stripAnsi(line));

  assert.ok(plain[0]!.includes("- 1"));
  assert.ok(plain[0]!.includes("const oldName = 1"));
  assert.ok(plain[1]!.includes("+ 1"));
  assert.ok(plain[1]!.includes("const newName = 1"));
  assert.ok(lines.some((line) => /\x1b\[/.test(line)));
});

test("renderColorDiffRows keeps compact marker and line-number gutters", () => {
  setNativeColorDiffForTest(null);
  const rows = renderColorDiffRows({
    filePath: "src/example.ts",
    width: 80,
    lines: [
      { kind: "context", oldLine: 4, newLine: 4, text: "const before = true" },
      { kind: "removed", oldLine: 5, text: "const oldName = 1" },
      { kind: "added", newLine: 5, text: "const newName = 1" },
    ],
  });
  const gutters = rows.map((row) => stripAnsi(row.gutter).trimEnd());

  assert.equal(gutters[0]!.trim(), "4");
  assert.equal(gutters[1]!.trim(), "- 5");
  assert.equal(gutters[2]!.trim(), "+ 5");
});

test("renderColorDiffRows fallback truncates by display width for CJK lines", () => {
  setNativeColorDiffForTest(null);
  const rows = renderColorDiffRows({
    filePath: "src/example.ts",
    width: 22,
    lines: [
      { kind: "removed", oldLine: 30, text: "成功使用file_write工具写入此行内容zheli shi ceshi wenben" },
      { kind: "added", newLine: 30, text: "nebnew ihsec ihs ilehz" },
    ],
  });

  assert.equal(stripAnsi(rows[0]!.gutter).trim(), "- 30");
  assert.equal(stripAnsi(rows[1]!.gutter).trim(), "+ 30");
  const contentWidth = Math.max(8, 22 - (String(30).length + 2) - 1);
  assert.ok(stringWidth(stripAnsi(rows[0]!.content)) <= contentWidth);
  assert.ok(stringWidth(stripAnsi(rows[1]!.content)) <= contentWidth);
});
