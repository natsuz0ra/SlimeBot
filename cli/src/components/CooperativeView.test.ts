import assert from "node:assert/strict";
import test from "node:test";
import { PassThrough } from "node:stream";
import React from "react";
import { Box, Text, renderSync, forceRedraw } from "ink";
import stripAnsi from "strip-ansi";
import CooperativeView, { CooperativeWorkspace } from "./CooperativeView.js";
import { terminalWorkspaceLayout } from "../utils/terminal.js";
import { Banner } from "./Banner.js";
import { cooperativeWorkspaceLines } from "../utils/cooperative.js";
import type { CooperativeSnapshot } from "../types/cooperative.js";

test("Ink keyboard flow preserves the view, submits commands, pages full reports and handles Escape", async () => {
  const snapshot: CooperativeSnapshot = {
    agents: [{ id: "agent-1", title: "Layout review", parentId: "root", profile: "reviewer", depth: 1, workspace: "/tmp/qa" }],
    turns: [{ id: "turn-1", agentId: "agent-1", status: "succeeded", answer: "Snapshot preview", createdAt: "2026-10-04T00:00:00Z", inputTokens: 20, outputTokens: 8 }],
    tasks: [{ id: "task-1", title: "Terminal layout", status: "pending", revision: 1, dependencies: "[]" }],
    artifacts: [],
    approvals: [],
  };
  const stdout = Object.assign(new PassThrough(), { isTTY: true, columns: 80, rows: 24, getWindowSize: () => [80, 24] });
  const stdin = Object.assign(new PassThrough(), { isTTY: true, setRawMode() {}, ref() {}, unref() {} });
  const stderr = new PassThrough();
  let output = "";
  stdout.on("data", chunk => { output += chunk.toString(); });
  const commands: string[] = [];
  let closes = 0;
  let refreshes = 0;
  let reports = 0;
  const instance = renderSync(React.createElement(CooperativeView, {
    snapshot, initialTab: 0, columns: 80, height: 18,
    refresh: () => { refreshes++; },
    onClose: () => { closes++; },
    onCommand: async command => { commands.push(command); },
    loadReport: async () => {
      reports++;
      return Array.from({ length: 200 }, (_, i) => `Full report line ${i}`).join("\n");
    },
  }), { stdout: stdout as never, stdin: stdin as never, stderr: stderr as never, exitOnCtrlC: false, patchConsole: false });
  async function press(key: string) {
    stdin.write(key);
    await new Promise(resolve => setTimeout(resolve, 120));
  }
  function paintedFrame() {
    // Normal Ink output is a cell diff. Request a complete paint before
    // reading text; blank cells are emitted as cursor moves rather than spaces.
    const start = output.length;
    forceRedraw(stdout as never);
    return stripAnsi(output.slice(start)).replace(/\s/g, "");
  }
  try {
    await press("\x1b[C");
    await press("r");
    assert.equal(refreshes, 1);
    assert.match(paintedFrame(), /\[Tasks1\]/);
    await press(":");
    await press("task create keyboard-check");
    await press("\r");
    assert.deepEqual(commands, ["task create keyboard-check"]);
    await press("\x1b[D");
    await press("\r");
    assert.equal(reports, 1);
    assert.match(paintedFrame(), /Fullreportline0/);
    await press("\x1b[F");
    assert.match(paintedFrame(), /Fullreportline199/);
    await press("\x1b[A");
    assert.match(paintedFrame(), /Lines191/);
    await press("\x1b");
    assert.equal(closes, 0, "first Escape returns from the report to the list");
    await press("\x1b");
    assert.equal(closes, 1, "second Escape closes the workspace even when Ink marks it as Meta");
  } finally {
    instance.unmount();
    instance.cleanup();
    stdin.destroy();
    stdout.destroy();
    stderr.destroy();
  }
});

test("actual Ink layout reserves room for the banner and context usage on short terminals", async () => {
  const { renderToScreen } = await import(new URL("../../packages/ink/src/ink/render-to-screen.ts", import.meta.url).href);
  const cwd = "/workspace/" + "a-long-project-directory/".repeat(6);
  const snapshot: CooperativeSnapshot = {
    agents: [{ id: "agent", title: "Layout", parentId: "root", profile: "reviewer", depth: 1, workspace: cwd }],
    turns: [{ id: "turn", agentId: "agent", status: "succeeded", createdAt: "2026-10-04T00:00:00Z", inputTokens: 1, outputTokens: 1, answer: "Full report\n".repeat(100) }],
    tasks: [], artifacts: [], approvals: [],
  };
  for (const [columns, rows] of [[30, 12], [40, 16], [80, 24], [120, 32]]) {
    const layout = terminalWorkspaceLayout(columns!, rows!, ["SlimeBot CLI 1.34.0 [new] [auto review]", "DeepSeek Flash", cwd], true);
    const lines = cooperativeWorkspaceLines({ snapshot, tab: 0, cursor: 0, columns: Math.min(columns!, 96), height: layout.height, detail: true });
    const banner = layout.compactBanner
      ? React.createElement(Text, { wrap: "truncate" }, "SlimeBot CLI 1.34.0 · DeepSeek Flash")
      : React.createElement(Banner, { version: "1.34.0", modelName: "DeepSeek Flash", cwd, approvalMode: "auto_review", updateAvailable: true });
    const result = renderToScreen(React.createElement(Box, { flexDirection: "column" },
      banner, React.createElement(Text, null, " "),
      React.createElement(Text, null, "Context usage"),
      React.createElement(Text, null, "─".repeat(columns!)),
      React.createElement(CooperativeWorkspace, { lines, columns: columns! }),
    ), columns!);
    assert.ok(result.height <= rows!, `${columns}×${rows} rendered ${result.height} rows`);
  }
});

test("a long command stays within a narrow terminal and Escape cancels without submission", async () => {
  const stdout = Object.assign(new PassThrough(), { isTTY: true, columns: 40, rows: 16, getWindowSize: () => [40, 16] });
  const stdin = Object.assign(new PassThrough(), { isTTY: true, setRawMode() {}, ref() {}, unref() {} });
  const stderr = new PassThrough();
  stdout.resume();
  let submissions = 0;
  let closes = 0;
  const overflowFrames: unknown[] = [];
  const instance = renderSync(React.createElement(CooperativeView, {
    snapshot: null, initialTab: 0, columns: 40, height: 16,
    refresh() {}, onClose: () => { closes++; },
    onCommand: async () => { submissions++; }, loadReport: async () => "",
  }), {
    stdout: stdout as never, stdin: stdin as never, stderr: stderr as never,
    exitOnCtrlC: false, patchConsole: false,
    onFrame: event => { overflowFrames.push(...event.flickers.filter(frame => frame.reason === "offscreen")); },
  });
  async function press(key: string) {
    stdin.write(key);
    await new Promise(resolve => setTimeout(resolve, 120));
  }
  try {
    await press(":");
    await press("task create " + "长命令🚀".repeat(100));
    await press("\x1b");
    assert.equal(submissions, 0);
    assert.equal(closes, 0);
    assert.deepEqual(overflowFrames, []);
    await press("\x1b");
    assert.equal(closes, 1);
  } finally {
    instance.unmount();
    instance.cleanup();
    stdin.destroy(); stdout.destroy(); stderr.destroy();
  }
});
