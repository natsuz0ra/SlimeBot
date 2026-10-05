import assert from "node:assert/strict";
import test from "node:test";
import { cooperativeRows, cooperativeWorkspaceLines, cooperativeReportPage, fitCooperativeText, latestCooperativeTurns } from "./cooperative.js";
import { stringWidth } from "./stringWidth.js";
import type { CooperativeSnapshot } from "../types/cooperative.js";
export function cooperativeFixture(): CooperativeSnapshot {
  return {
    agents: Array.from({ length: 12 }, (_, index) => ({ id: `agent-${index}`, title: `研究代理 ${index} · 终端长标题适配 🚀`, profile: "researcher", parentId: "root", depth: 1, workspace: "/workspace/project", task: "Review layout and keyboard interaction" })),
    turns: [{ id: "latest", agentId: "agent-0", status: "failed", createdAt: "2026-10-04T12:00:00Z", inputTokens: 20, outputTokens: 8, answer: "最新完整报告\n" + Array.from({ length: 100 }, (_, i) => `Line ${i} · 中英文 mixed content 🚀`).join("\n") }, { id: "older", agentId: "agent-0", status: "running", createdAt: "2026-10-03T12:00:00Z", inputTokens: 10, outputTokens: 2 }],
    tasks: [{ id: "task-1", title: "Check terminal layout", status: "in_progress", revision: 2, dependencies: "[]", description: "Verify resize, scroll and approval flow" }],
    artifacts: [{ id: "file-1", agentId: "agent-0", status: "ready", workspace: "/workspace/project", report: "File changes are ready for verification." }],
    approvals: Array.from({ length: 15 }, (_, index) => ({ id: `approval-${index}`, agentId: "agent-0", status: "pending", payload: JSON.stringify({ toolName: "exec", command: "run", params: { command: "go test ./..." } }) })),
  };
}
test("latest agent state does not depend on server array order", () => {
  const fixture = cooperativeFixture();
  assert.equal(latestCooperativeTurns(fixture.turns).get("agent-0")?.status, "failed");
  assert.equal(cooperativeRows(fixture, 0)[0]?.turnId, "latest");
});
test("approval tab provides the full request while bounding the list", () => {
  const fixture = cooperativeFixture();
  const rows = cooperativeRows(fixture, 3);
  assert.equal(rows.length, 15);
  assert.match(rows[0]!.report, /go test \.\/\.\.\./);
  const frame = cooperativeWorkspaceLines({ snapshot: fixture, tab: 3, cursor: 14, columns: 40, height: 16 });
  assert.ok(frame.some(line => line.text.includes("15/15")));
  assert.ok(frame.some(line => line.text.includes("approval-14")));
  assert.ok(frame.length <= 16);
});
test("all workspace tabs and reports fit narrow and wide terminal frames", () => {
  const fixture = cooperativeFixture();
  for (const columns of [30, 40, 80, 120])
    for (const height of [12, 18, 32])
      for (const tab of [0, 1, 2, 3])
        for (const detail of [false, true]) {
          const width = Math.min(columns, 96);
          const lines = cooperativeWorkspaceLines({ snapshot: fixture, tab, cursor: 0, columns: width, height, detail, offset: 99 });
          assert.ok(lines.length <= height, `${columns}×${height}, tab ${tab}`);
          for (const line of lines)
            assert.ok(stringWidth(line.text) <= width - 2, `${columns}: ${line.text}`);
        }
});
test("report paging preserves complete content and can scroll back from the end", () => {
  const report = "报告\n" + Array.from({ length: 200 }, (_, i) => `Row ${i}`).join("\n");
  const end = cooperativeReportPage(report, 40, 8, Number.MAX_SAFE_INTEGER);
  assert.equal(end.lines.at(-1), "Row 199");
  const previous = cooperativeReportPage(report, 40, 8, end.offset - 1);
  assert.equal(previous.offset, end.offset - 1);
  assert.equal(cooperativeReportPage(report, 40, 8, 0).lines[0], "报告");
});
test("terminal labels keep graphemes intact and neutralize control sequences", () => {
  assert.equal(fitCooperativeText("👩‍💻👩‍💻👩‍💻", 5), "👩‍💻👩‍💻…");
  assert.equal(fitCooperativeText("标题\n\x1b[31m红色\x1b[0m", 30), "标题 红色");
});
