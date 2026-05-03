import assert from "node:assert/strict";
import test from "node:test";
import { dispatchWSMessage, type WSHandlers } from "./socket";

test("dispatchWSMessage routes plan_chunk with content and session id", () => {
  const calls: Array<{ chunk: string; sessionId?: string }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onPlanChunk: (chunk, sessionId) => {
      calls.push({ chunk, sessionId });
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "plan_chunk",
      sessionId: "sid-1",
      content: "# Plan\n",
    }),
    handlers,
  );

  assert.deepEqual(calls, [{ chunk: "# Plan\n", sessionId: "sid-1" }]);
});

test("dispatchWSMessage routes done plan confirmation metadata", () => {
  const calls: Array<{
    sessionId?: string;
    meta?: { planId?: string; planBody?: string; narration?: string };
  }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: (sessionId, meta) => {
      calls.push({ sessionId, meta });
    },
    onError: () => {},
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "done",
      sessionId: "sid-2",
      planId: "plan-1",
      planBody: "# Plan\n\nDo it.",
      narration: "before",
    }),
    handlers,
  );

  assert.deepEqual(calls, [{
    sessionId: "sid-2",
    meta: {
      isInterrupted: undefined,
      isStopPlaceholder: undefined,
      planId: "plan-1",
      planBody: "# Plan\n\nDo it.",
      narration: "before",
    },
  }]);
});

test("dispatchWSMessage does not synthesize plan confirmation from plan_body", () => {
  let doneCalls = 0;
  let planBodyCalls = 0;
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {
      doneCalls += 1;
    },
    onError: () => {},
    onPlanBody: () => {
      planBodyCalls += 1;
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "plan_body",
      sessionId: "sid-3",
      content: "# Plan\n\nDraft",
    }),
    handlers,
  );

  assert.equal(doneCalls, 0);
  assert.equal(planBodyCalls, 1);
});

test("dispatchWSMessage routes todo_update with items, note, and session id", () => {
  const calls: Array<{
    sessionId?: string;
    items: Array<{ id: string; content: string; status: string }>;
    note?: string;
  }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onTodoUpdate: (data, sessionId) => {
      calls.push({ sessionId, items: data.items, note: data.note });
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "todo_update",
      sessionId: "sid-todo",
      note: "working",
      items: [
        { id: "inspect", content: "Inspect current flow", status: "completed" },
        { id: "store", content: "Update store", status: "in_progress" },
      ],
    }),
    handlers,
  );

  assert.deepEqual(calls, [{
    sessionId: "sid-todo",
    note: "working",
    items: [
      { id: "inspect", content: "Inspect current flow", status: "completed" },
      { id: "store", content: "Update store", status: "in_progress" },
    ],
  }]);
});

test("dispatchWSMessage routes subagent_start with title and task", () => {
  const calls: Array<{
    sessionId?: string;
    title: string;
    task: string;
  }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onSubagentStart: (data, sessionId) => {
      calls.push({ sessionId, title: data.title, task: data.task });
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "subagent_start",
      sessionId: "sid-sub",
      parentToolCallId: "parent-tool",
      subagentRunId: "run-1",
      title: "Inspect UI cards",
      task: "Inspect UI cards and report exact files",
    }),
    handlers,
  );

  assert.deepEqual(calls, [{
    sessionId: "sid-sub",
    title: "Inspect UI cards",
    task: "Inspect UI cards and report exact files",
  }]);
});

test("dispatchWSMessage routes subagent_done with error", () => {
  const calls: Array<{
    sessionId?: string;
    parentToolCallId: string;
    subagentRunId: string;
    error?: string;
  }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onSubagentDone: (data, sessionId) => {
      calls.push({ sessionId, ...data });
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "subagent_done",
      sessionId: "sid-sub",
      parentToolCallId: "parent-tool",
      subagentRunId: "run-1",
      error: "context canceled",
    }),
    handlers,
  );

  assert.deepEqual(calls, [{
    sessionId: "sid-sub",
    parentToolCallId: "parent-tool",
    subagentRunId: "run-1",
    error: "context canceled",
  }]);
});

test("dispatchWSMessage routes context usage events", () => {
  const calls: Array<{ sessionId?: string; usedPercent: number; isCompacted: boolean }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onContextUsage: (usage, sessionId) => {
      calls.push({ sessionId, usedPercent: usage.usedPercent, isCompacted: usage.isCompacted });
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "context_usage",
      sessionId: "sid-context",
      modelConfigId: "model-1",
      usedTokens: 420000,
      totalTokens: 1000000,
      usedPercent: 42,
      availablePercent: 58,
      isCompacted: true,
    }),
    handlers,
  );

  assert.deepEqual(calls, [{ sessionId: "sid-context", usedPercent: 42, isCompacted: true }]);
});

test("dispatchWSMessage routes context compacted events", () => {
  const calls: Array<{ sessionId?: string; usedTokens: number }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onContextCompacted: (usage, sessionId) => {
      calls.push({ sessionId, usedTokens: usage.usedTokens });
    },
  };

  dispatchWSMessage(
    JSON.stringify({
      type: "context_compacted",
      sessionId: "sid-context",
      usage: {
        sessionId: "sid-context",
        modelConfigId: "model-1",
        usedTokens: 120000,
        totalTokens: 500000,
        usedPercent: 24,
        availablePercent: 76,
        isCompacted: true,
      },
    }),
    handlers,
  );

  assert.deepEqual(calls, [{ sessionId: "sid-context", usedTokens: 120000 }]);
});
