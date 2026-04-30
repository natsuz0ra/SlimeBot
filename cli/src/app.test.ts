import assert from "node:assert/strict";
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
    },
    { kind: "assistant", content: "running tool" },
  ]);
});

test("getChatFooterHint returns toggle hint in plan mode", () => {
  assert.equal(
    getChatFooterHint(true, "standard"),
    "/ for commands | Shift+Tab to toggle | Esc to cancel",
  );
});

test("getChatFooterHint returns toggle hint in auto mode", () => {
  assert.equal(
    getChatFooterHint(false, "auto"),
    "/ for commands | Shift+Tab to toggle | Esc to cancel",
  );
});

test("getChatFooterHint returns default hint in standard mode", () => {
  assert.equal(
    getChatFooterHint(false, "standard"),
    "/ for commands | Shift+Tab plan mode | Esc to cancel",
  );
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
  assert.equal(parent.subagentThinking?.thinkingDone, true);
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
