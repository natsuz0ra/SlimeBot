import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import type { Key } from "ink";
import { getChatFooterHint, handleChatShortcut } from "./app";
import { mapHistoryMessages } from "./utils/history";
import type { Message, ThinkingHistoryItem, ToolCallHistoryItem } from "./types";

test("mapHistoryMessages inserts tool calls after assistant messages in timeline order", () => {
  const messages: Message[] = [
    {
      id: "u1",
      sessionId: "s1",
      role: "user",
      content: "hello",
      seq: 1,
      createdAt: "2026-01-01T00:00:00Z",
    },
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: "running tool",
      seq: 2,
      createdAt: "2026-01-01T00:00:01Z",
    },
    {
      id: "a-stop",
      sessionId: "s1",
      role: "assistant",
      content: "placeholder",
      seq: 3,
      isStopPlaceholder: true,
      createdAt: "2026-01-01T00:00:02Z",
    },
  ];

  const toolCallsByMsgId: Record<string, ToolCallHistoryItem[]> = {
    a1: [
      {
        toolCallId: "tc1",
        toolName: "web_search",
        command: "search",
        params: { q: "hello" },
        status: "completed",
        requiresApproval: false,
        output: "result",
        startedAt: "2026-01-01T00:00:01Z",
        finishedAt: "2026-01-01T00:00:02Z",
      },
    ],
  };

  const entries = mapHistoryMessages(messages, toolCallsByMsgId);

  assert.deepEqual(entries, [
    { kind: "user", content: "hello" },
    {
      kind: "tool",
      content: "result",
      toolCallId: "tc1",
      toolName: "web_search",
      command: "search",
      params: { q: "hello" },
      status: "completed",
      output: "result",
      error: undefined,
      metadata: undefined,
    },
    { kind: "assistant", content: "running tool" },
  ]);
});

test("mapHistoryMessages restores only explicit Agent Team history", () => {
  const messages: Message[] = [{
    id: "a1", sessionId: "s1", role: "assistant", content: "done", seq: 1, createdAt: "2026-07-29T00:00:05Z",
  }];
  const entries = mapHistoryMessages(messages, {}, {}, [{
    id: "team-1", sessionId: "s1", requestId: "request-1", assistantMessageId: "a1", status: "succeeded",
    maxMembers: 8, maxParallel: 4, startedAt: "2026-07-29T00:00:00Z", finishedAt: "2026-07-29T00:00:04Z",
  }], [{
    id: "member-1", teamRunId: "team-1", toolCallId: "tool-1", title: "Research", task: "Inspect", status: "succeeded",
    createdAt: "2026-07-29T00:00:01Z", finishedAt: "2026-07-29T00:00:03Z",
  }]);

  assert.equal(entries.filter((entry) => entry.kind === "team").length, 1);
  assert.equal(entries.find((entry) => entry.kind === "team")?.teamRun?.members[0]?.id, "member-1");
  assert.equal(mapHistoryMessages(messages, {}, {}).some((entry) => entry.kind === "team"), false);
});

test("getChatFooterHint returns toggle hint in plan mode", () => {
  assert.equal(
    getChatFooterHint(true, "standard"),
    "/ for commands · Shift+Tab toggle · Esc cancel",
  );
});

test("getChatFooterHint returns toggle hint in auto mode", () => {
  assert.equal(
    getChatFooterHint(false, "auto"),
    "/ for commands · Shift+Tab toggle · Esc cancel",
  );
});

test("getChatFooterHint returns toggle hint in auto review mode", () => {
  assert.equal(
    getChatFooterHint(false, "auto_review"),
    "/ for commands · Shift+Tab toggle · Esc cancel",
  );
});

// Memory console behavior is covered by controllers/memoryConsole.test.ts.


test("Agent Team command opens a dedicated compact detail view", () => {
  const source = readFileSync(new URL("./app.tsx", import.meta.url), "utf-8");

  assert.match(source, /openTeamDetail:\s*\(\)\s*=>\s*dispatch\(\{\s*type:\s*"OPEN_TEAM_DETAIL"/);
  assert.match(source, /<TeamView/);
  assert.match(source, /teamRunCursor/);
  assert.match(source, /teamMemberCursor/);
  assert.match(source, /←\/→ Team · ↑\/↓ member · Esc back/);
});

test("getChatFooterHint returns default hint in standard mode", () => {
  assert.equal(
    getChatFooterHint(false, "standard"),
    "/ for commands · Shift+Tab plan · Esc cancel",
  );
});

test("app footer operation hints use the shared slate hint color", () => {
  const source = readFileSync(new URL("./app.tsx", import.meta.url), "utf8");

  assert.doesNotMatch(source, /color="gray" dimColor/);
});

test("sandbox menu persists CLI-specific sandbox settings", () => {
  const source = readFileSync(new URL("./app.tsx", import.meta.url), "utf8");

  assert.match(source, /updateSettings\(\{ cliSandboxMode: action\.mode \}\)/);
  assert.match(source, /updateSettings\(\{ cliSandboxNetworkEnabled: action\.enabled \}\)/);
  assert.doesNotMatch(source, /updateSettings\(\{ sandboxMode: action\.mode \}\)/);
  assert.doesNotMatch(source, /updateSettings\(\{ sandboxNetworkEnabled: action\.enabled \}\)/);
});

test("CLI entry lets the app handle Ctrl+C shortcuts", () => {
  const source = readFileSync(new URL("./index.tsx", import.meta.url), "utf8");

  assert.match(source, /exitOnCtrlC:\s*false/);
});

test("CLI release bundle opts runtime dependencies back into tsup bundling", () => {
  const source = readFileSync(new URL("../tsup.config.ts", import.meta.url), "utf8");

  assert.match(source, /noExternal:\s*\[/);
  assert.match(source, /shims:\s*true/);
  assert.match(source, /__slimebotCreateRequire\(import\.meta\.url\)/);
  assert.match(source, /@slimebot\\\/color-diff-native/);
  assert.match(source, /external:\s*\[\s*["']@slimebot\/color-diff-native["']\s*\]/);
  assert.match(source, /options\.alias\s*=\s*\{/);
  assert.match(source, /["']react["']:\s*resolve\(__dirname,\s*["']node_modules\/react\/index\.js["']\)/);
  assert.match(source, /["']react\/jsx-runtime["']:\s*resolve\(__dirname,\s*["']node_modules\/react\/jsx-runtime\.js["']\)/);
  assert.match(source, /["']react\/compiler-runtime["']:\s*resolve\(__dirname,\s*["']node_modules\/react\/compiler-runtime\.js["']\)/);
  assert.match(source, /["']react-reconciler["']:\s*resolve\(__dirname,\s*["']node_modules\/react-reconciler\/index\.js["']\)/);
  assert.match(source, /["']react-reconciler\/constants\.js["']:\s*resolve\(__dirname,\s*["']node_modules\/react-reconciler\/constants\.js["']\)/);
});

test("release packaging fails when CLI bundle still imports runtime packages", () => {
  const source = readFileSync(new URL("../../scripts/package-release.sh", import.meta.url), "utf8");

  assert.match(source, /assert_cli_bundle_self_contained/);
  assert.match(source, /grep -Fq/);
  assert.match(source, /react\/jsx-runtime/);
  assert.match(source, /packages\/ink\/node_modules\/react/);
  assert.match(source, /react_module_count/);
  assert.match(source, /"node_modules\/react\/index\.js"\(exports, module\)/);
  assert.match(source, /assert_cli_bundle_self_contained "cli\/dist\/index\.js"/);
});

test("session redraw uses Ink frame reset instead of raw terminal clearing", () => {
  const source = readFileSync(new URL("./app.tsx", import.meta.url), "utf8");

  assert.match(source, /forceRedraw\(stdout,\s*\{\s*clearScrollback:\s*true\s*\}\)/);
  assert.match(source, /forceRedraw\(stdout,\s*\{\s*clearScrollback:\s*true\s*\}\)[\s\S]*clearScreen\(\)/);
  assert.doesNotMatch(source, /const clearScreenDeferred = useCallback/);
});

test("app passes update availability to the banner", () => {
  const source = readFileSync(new URL("./app.tsx", import.meta.url), "utf8");

  assert.match(source, /updateAvailable=\{Boolean\(state\.updateCheck\?\.updateAvailable\)\}/);
});

test("internal Ink forceRedraw keeps scrollback optional", () => {
  const inkSource = readFileSync(new URL("../packages/ink/src/ink/ink.tsx", import.meta.url), "utf8");
  const rootSource = readFileSync(new URL("../packages/ink/src/ink/root.ts", import.meta.url), "utf8");

  assert.match(inkSource, /export type ForceRedrawOptions = \{\s*clearScrollback\?: boolean\s*\}/);
  assert.match(inkSource, /options\.clearScrollback \? clearTerminal : ERASE_SCREEN \+ CURSOR_HOME/);
  assert.match(rootSource, /options: ForceRedrawOptions = \{\}/);
  assert.match(rootSource, /instance\.forceRedraw\(options\)/);
});

test("mapHistoryMessages preserves parentToolCallId for nested tool calls", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: "done",
      seq: 1,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const toolCallsByMsgId: Record<string, ToolCallHistoryItem[]> = {
    a1: [
      {
        toolCallId: "parent",
        toolName: "run_subagent",
        command: "delegate",
        params: { task: "x" },
        status: "completed",
        requiresApproval: false,
        output: "ok",
        startedAt: "2026-01-01T00:00:00Z",
      },
      {
        toolCallId: "child",
        toolName: "web_search",
        command: "search",
        params: { q: "y" },
        status: "completed",
        requiresApproval: false,
        parentToolCallId: "parent",
        subagentRunId: "run-1",
        output: "hits",
        startedAt: "2026-01-01T00:00:01Z",
      },
    ],
  };
  const entries = mapHistoryMessages(messages, toolCallsByMsgId);
  const child = entries.find((e) => e.kind === "tool" && e.toolCallId === "child");
  assert.ok(child && child.kind === "tool");
  assert.equal(child.parentToolCallId, "parent");
  assert.equal(child.subagentRunId, "run-1");
});

test("mapHistoryMessages restores run_subagent title from history params", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: "done",
      seq: 1,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const toolCallsByMsgId: Record<string, ToolCallHistoryItem[]> = {
    a1: [
      {
        toolCallId: "parent",
        toolName: "run_subagent",
        command: "delegate",
        params: { title: "Inspect UI cards", task: "Inspect UI cards and report exact files" },
        status: "completed",
        requiresApproval: false,
        output: "ok",
        startedAt: "2026-01-01T00:00:00Z",
      },
    ],
  };

  const entries = mapHistoryMessages(messages, toolCallsByMsgId);
  const parent = entries.find((e) => e.kind === "tool" && e.toolCallId === "parent");

  assert.ok(parent && parent.kind === "tool");
  assert.equal(parent.subagentTitle, "Inspect UI cards");
  assert.equal(parent.subagentTask, "Inspect UI cards and report exact files");
});

test("mapHistoryMessages marks interrupted open subagent tool history as error", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: "<!-- TOOL_CALL:parent -->",
      seq: 1,
      isInterrupted: true,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const toolCallsByMsgId: Record<string, ToolCallHistoryItem[]> = {
    a1: [
      {
        toolCallId: "parent",
        toolName: "run_subagent",
        command: "delegate",
        params: { task: "x" },
        status: "executing",
        requiresApproval: false,
        startedAt: "2026-01-01T00:00:00Z",
      },
    ],
  };

  const entries = mapHistoryMessages(messages, toolCallsByMsgId);
  const parent = entries.find((e) => e.kind === "tool" && e.toolCallId === "parent");

  assert.ok(parent && parent.kind === "tool");
  assert.equal(parent.status, "error");
  assert.equal(parent.error, "Execution cancelled.");
});

test("mapHistoryMessages closes interrupted streaming subagent thinking history", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: "<!-- TOOL_CALL:parent -->",
      seq: 1,
      isInterrupted: true,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const toolCallsByMsgId: Record<string, ToolCallHistoryItem[]> = {
    a1: [
      {
        toolCallId: "parent",
        toolName: "run_subagent",
        command: "delegate",
        params: { task: "x" },
        status: "error",
        error: "Execution cancelled.",
        requiresApproval: false,
        startedAt: "2026-01-01T00:00:00Z",
      },
    ],
  };
  const thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {
    a1: [
      {
        thinkingId: "think-child",
        parentToolCallId: "parent",
        subagentRunId: "sub-run",
        content: "child reasoning",
        status: "streaming",
        startedAt: "2026-01-01T00:00:00Z",
      },
    ],
  };

  const entries = mapHistoryMessages(messages, toolCallsByMsgId, thinkingByMsgId);
  const parent = entries.find((e) => e.kind === "tool" && e.toolCallId === "parent");

  assert.ok(parent && parent.kind === "tool");
  assert.equal(parent.subagentThinkings?.at(-1)?.thinkingDone, true);
});

test("mapHistoryMessages restores thinking entries from history markers", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: "<!-- THINKING:think-1 -->\nDone",
      seq: 1,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {
    a1: [
      {
        thinkingId: "think-1",
        content: "reasoning text",
        status: "completed",
        startedAt: "2026-01-01T00:00:00Z",
        finishedAt: "2026-01-01T00:00:01Z",
        durationMs: 1000,
      },
    ],
  };

  const entries = mapHistoryMessages(messages, {}, thinkingByMsgId);

  assert.deepEqual(entries, [
    {
      kind: "thinking",
      content: "reasoning text",
      thinkingDone: true,
      thinkingDurationMs: 1000,
      thinkingStartedAt: Date.parse("2026-01-01T00:00:00Z"),
    },
    { kind: "assistant", content: "Done" },
  ]);
});

test("mapHistoryMessages restores thinking markers inside plan history in marker order", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: [
        "<!-- PLAN_START -->",
        "<!-- THINKING:think-in-plan -->",
        "# Plan",
        "",
        "Do the thing.",
        "<!-- PLAN_END -->",
      ].join("\n"),
      seq: 1,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {
    a1: [
      {
        thinkingId: "think-in-plan",
        content: "internal plan reasoning",
        status: "completed",
        startedAt: "2026-01-01T00:00:00Z",
        finishedAt: "2026-01-01T00:00:01Z",
        durationMs: 1000,
      },
    ],
  };

  const entries = mapHistoryMessages(messages, {}, thinkingByMsgId);

  assert.deepEqual(entries.map((entry) => entry.kind), ["thinking", "plan"]);
  assert.equal(entries[0].content, "internal plan reasoning");
  assert.equal(entries[1].content, "# Plan\n\nDo the thing.");
});

test("mapHistoryMessages keeps thinking outside and inside a plan in order", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: [
        "<!-- THINKING:think-before -->",
        "Preamble",
        "<!-- PLAN_START -->",
        "<!-- THINKING:think-in-plan -->",
        "# Plan",
        "<!-- PLAN_END -->",
        "<!-- THINKING:think-after -->",
        "Done",
      ].join("\n"),
      seq: 1,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {
    a1: [
      {
        thinkingId: "think-before",
        content: "before reasoning",
        status: "completed",
        startedAt: "2026-01-01T00:00:00Z",
        durationMs: 1000,
      },
      {
        thinkingId: "think-in-plan",
        content: "internal plan reasoning",
        status: "completed",
        startedAt: "2026-01-01T00:00:01Z",
        durationMs: 1000,
      },
      {
        thinkingId: "think-after",
        content: "after reasoning",
        status: "completed",
        startedAt: "2026-01-01T00:00:02Z",
        durationMs: 1000,
      },
    ],
  };

  const entries = mapHistoryMessages(messages, {}, thinkingByMsgId);

  assert.deepEqual(entries.map((entry) => entry.kind), ["thinking", "assistant", "thinking", "plan", "thinking", "assistant"]);
  assert.equal(entries.filter((entry) => entry.kind === "thinking").length, 3);
  assert.equal(entries.find((entry) => entry.kind === "plan")?.content, "# Plan");
});

test("mapHistoryMessages restores text, thinking, and plan markers without dropping plan thinking", () => {
  const messages: Message[] = [
    {
      id: "a1",
      sessionId: "s1",
      role: "assistant",
      content: [
        "<!-- THINKING:think-1 -->",
        "Narration",
        "<!-- THINKING:think-2 -->",
        "<!-- PLAN_START -->",
        "<!-- THINKING:think-3 -->",
        "# Plan",
        "",
        "Do the thing.",
        "<!-- PLAN_END -->",
      ].join("\n"),
      seq: 1,
      createdAt: "2026-01-01T00:00:01Z",
    },
  ];
  const thinkingByMsgId: Record<string, ThinkingHistoryItem[]> = {
    a1: [
      {
        thinkingId: "think-1",
        content: "first thought",
        status: "completed",
        startedAt: "2026-01-01T00:00:00Z",
        durationMs: 1000,
      },
      {
        thinkingId: "think-2",
        content: "second thought",
        status: "completed",
        startedAt: "2026-01-01T00:00:01Z",
        durationMs: 1000,
      },
      {
        thinkingId: "think-3",
        content: "plan thought",
        status: "completed",
        startedAt: "2026-01-01T00:00:02Z",
        durationMs: 1000,
      },
    ],
  };

  const entries = mapHistoryMessages(messages, {}, thinkingByMsgId);

  assert.deepEqual(entries.map((entry) => entry.kind), ["thinking", "assistant", "thinking", "thinking", "plan"]);
  assert.equal(entries[0].content, "first thought");
  assert.equal(entries[1].content, "Narration");
  assert.equal(entries[2].content, "second thought");
  assert.equal(entries[3].content, "plan thought");
  assert.equal(entries[4].content, "# Plan\n\nDo the thing.");
});

function key(overrides: Partial<Key> = {}): Key {
  return {
    upArrow: false,
    downArrow: false,
    leftArrow: false,
    rightArrow: false,
    pageDown: false,
    pageUp: false,
    home: false,
    end: false,
    return: false,
    escape: false,
    ctrl: false,
    shift: false,
    tab: false,
    backspace: false,
    delete: false,
    meta: false,
    ...overrides,
  } as Key;
}

test("handleChatShortcut handles Ctrl+O and raw Ctrl+O", () => {
  const actions: string[] = [];
  const dispatch = (action: { type: string }) => {
    actions.push(action.type);
    return action as any;
  };

  const handledNormal = handleChatShortcut("o", key({ ctrl: true }), dispatch as any);
  const handledRaw = handleChatShortcut(String.fromCharCode(15), key(), dispatch as any);

  assert.equal(handledNormal, true);
  assert.equal(handledRaw, true);
  assert.equal(actions.filter((x) => x === "TOGGLE_TOOL_OUTPUT").length, 2);
});
