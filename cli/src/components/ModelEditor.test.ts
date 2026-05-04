import assert from "node:assert/strict";
import test from "node:test";
import {
  formatContextSizeDisplay,
  formatModelFieldValue,
  maskModelApiKey,
  truncateDisplayValue,
} from "./ModelEditor";

test("truncateDisplayValue truncates long values with an ellipsis", () => {
  assert.equal(truncateDisplayValue("https://example.com/v1/chat/completions", 16), "https://example…");
});

test("formatModelFieldValue renders empty values consistently", () => {
  assert.equal(formatModelFieldValue("", 20), "(empty)");
  assert.equal(formatModelFieldValue("   ", 20), "(empty)");
});

test("maskModelApiKey does not expose the original key", () => {
  const masked = maskModelApiKey("sk-test-secret-value-that-should-not-render");

  assert.equal(masked, "********************");
  assert.equal(masked.includes("sk-test"), false);
});

test("formatContextSizeDisplay clamps and formats context size", () => {
  assert.equal(formatContextSizeDisplay("1"), "8K");
});
