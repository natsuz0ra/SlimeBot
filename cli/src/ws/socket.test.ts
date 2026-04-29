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
