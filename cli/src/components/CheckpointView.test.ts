import assert from "node:assert/strict";
import test from "node:test";
import { parseUnifiedDiffLines } from "./CheckpointView.js";

test("parseUnifiedDiffLines extracts line numbers and change kinds from unified diff", () => {
  const diff = [
    "diff --git a/src/app.ts b/src/app.ts",
    "index 1111111..2222222 100644",
    "--- a/src/app.ts",
    "+++ b/src/app.ts",
    "@@ -10,4 +10,4 @@ function demo() {",
    " const a = 1;",
    "-const b = 2;",
    "+const b = 3;",
    " return a + b;",
  ].join("\n");

  const lines = parseUnifiedDiffLines(diff);
  assert.deepEqual(lines, [
    { kind: "context", oldLine: 10, newLine: 10, text: "const a = 1;" },
    { kind: "removed", oldLine: 11, text: "const b = 2;" },
    { kind: "added", newLine: 11, text: "const b = 3;" },
    { kind: "context", oldLine: 12, newLine: 12, text: "return a + b;" },
  ]);
});
