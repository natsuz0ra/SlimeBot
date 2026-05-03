import assert from "node:assert/strict";
import test from "node:test";
import {
  CONTEXT_SIZE_DEFAULT,
  CONTEXT_SIZE_MAX,
  CONTEXT_SIZE_MIN,
  adjustContextSize,
  clampContextSize,
  formatContextUsageStatus,
  formatContextSize,
  formatContextTokenCount,
  renderContextSizeBar,
} from "./contextSize";

test("clampContextSize keeps CLI values in range", () => {
  assert.equal(clampContextSize(1_000), CONTEXT_SIZE_MIN);
  assert.equal(clampContextSize(2_000_000), CONTEXT_SIZE_MAX);
  assert.equal(clampContextSize("bad"), CONTEXT_SIZE_DEFAULT);
  assert.equal(clampContextSize("64000"), 64_000);
});

test("formatContextSize renders compact CLI labels", () => {
  assert.equal(formatContextSize(8_000), "8K");
  assert.equal(formatContextSize(128_000), "128K");
  assert.equal(formatContextSize(1_000_000), "1M");
});

test("formatContextTokenCount uses token, k, and m units", () => {
  assert.equal(formatContextTokenCount(987), "987 tokens");
  assert.equal(formatContextTokenCount(23_700), "23.7k tokens");
  assert.equal(formatContextTokenCount(1_240_000), "1.2m tokens");
});

test("formatContextUsageStatus degrades for narrow terminals", () => {
  const usage = {
    sessionId: "sid-1",
    modelConfigId: "model-1",
    usedTokens: 420_000,
    totalTokens: 1_000_000,
    usedPercent: 42,
    availablePercent: 58,
    isCompacted: true,
  };

  assert.equal(formatContextUsageStatus(usage, 80), "CTX 42% · 420.0k tokens/1.0m tokens · compacted");
  assert.equal(formatContextUsageStatus(usage, 18), "CTX 42%");
});

test("adjustContextSize clamps keyboard deltas", () => {
  assert.equal(adjustContextSize(8_000, -1_000), CONTEXT_SIZE_MIN);
  assert.equal(adjustContextSize(999_500, 1_000), CONTEXT_SIZE_MAX);
});

test("renderContextSizeBar returns stable width", () => {
  assert.equal(renderContextSizeBar(8_000, 10), "----------");
  assert.equal(renderContextSizeBar(1_000_000, 10), "==========");
  assert.equal(renderContextSizeBar(128_000, 10).length, 10);
});
