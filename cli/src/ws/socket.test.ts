import assert from "node:assert/strict";
import { EventEmitter, once } from "node:events";
import test, { type TestContext } from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { WebSocketServer, type WebSocket } from "ws";
import { CLISocket, dispatchWSMessage, type WSHandlers } from "./socket";

async function startSocketServer(t: TestContext) {
  const server = new WebSocketServer({ host: "127.0.0.1", port: 0 });
  const client = new CLISocket();
  const watches = new EventEmitter();
  server.on("connection", (peer) => {
    peer.on("message", (data) => {
      const msg = JSON.parse(data.toString());
      if (msg.type === "watch_session") watches.emit("watch", msg.sessionId);
    });
  });
  t.after(async () => {
    client.close();
    for (const peer of server.clients) peer.terminate();
    await new Promise<void>((resolve) => server.close(() => resolve()));
  });
  await once(server, "listening");
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  return { server, client, watches, url: `http://127.0.0.1:${address.port}` };
}

const noOpHandlers: WSHandlers = {
  onSession: () => {},
  onStart: () => {},
  onChunk: () => {},
  onDone: () => {},
  onError: () => {},
};

test("unexpected close clears the turn and reconnects watching the latest session", { timeout: 5_000 }, async (t) => {
  const { server, client, watches, url } = await startSocketServer(t);
  const opens: boolean[] = [];
  let disconnects = 0;
  const connected = once(server, "connection");
  const firstWatch = once(watches, "watch");
  client.connect(url, "token", {
    ...noOpHandlers,
    onOpen: (reconnected) => opens.push(reconnected),
    onClose: () => {
      disconnects++;
      assert.equal(client.activeTurnId, "");
      assert.equal(client.watchSession("b"), false);
    },
  });
  client.watchSession("a");
  const [peer] = await connected as [WebSocket];
  assert.deepEqual(await firstWatch, ["a"]);

  client.activeTurnId = "lost-turn";
  const secondWatch = once(watches, "watch");
  peer.terminate();
  assert.deepEqual(await secondWatch, ["b"]);
  assert.deepEqual(opens, [false, true]);
  assert.equal(disconnects, 1);
});

test("active close does not notify a disconnect or reconnect", { timeout: 5_000 }, async (t) => {
  const { server, client, url } = await startSocketServer(t);
  let connections = 0;
  let disconnects = 0;
  server.on("connection", () => { connections++; });
  const connected = once(server, "connection");
  client.connect(url, "token", { ...noOpHandlers, onClose: () => { disconnects++; } });
  const [peer] = await connected as [WebSocket];
  const closed = once(peer, "close");
  client.close();
  await closed;
  await delay(350);
  assert.equal(disconnects, 0);
  assert.equal(connections, 1);
});

test("active close cancels a scheduled reconnect", { timeout: 5_000 }, async (t) => {
  const { server, client, url } = await startSocketServer(t);
  let connections = 0;
  server.on("connection", () => { connections++; });
  const connected = once(server, "connection");
  const events = new EventEmitter();
  const disconnected = once(events, "disconnect");
  client.connect(url, "token", { ...noOpHandlers, onClose: () => events.emit("disconnect") });
  const [peer] = await connected as [WebSocket];
  peer.terminate();
  await disconnected;
  client.close();
  await delay(350);
  assert.equal(connections, 1);
});

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
    onDone: (sessionId, _answer, meta) => {
      calls.push({ sessionId, meta: { planId: meta?.planId, planBody: meta?.planBody, narration: meta?.narration } });
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

test("dispatchWSMessage routes auto approval review lifecycle events", () => {
  const reviews: Array<{ status: string; reason?: string }> = [];
  const required: Array<{ toolCallId: string; params: Record<string, unknown>; reason?: string }> = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onToolCallReview: (data) => {
      reviews.push({ status: data.reviewStatus, reason: data.reviewReason });
    },
    onToolApprovalRequired: (data) => {
      required.push({ toolCallId: data.toolCallId, params: data.params, reason: data.reviewReason });
    },
  };

  dispatchWSMessage(JSON.stringify({
    type: "tool_call_review",
    toolCallId: "call-1",
    toolName: "exec",
    command: "run",
    reviewStatus: "reviewing",
  }), handlers);
  dispatchWSMessage(JSON.stringify({
    type: "tool_call_approval_required",
    toolCallId: "call-1",
    toolName: "exec",
    command: "run",
    params: { command: "rm -rf /tmp/example" },
    requiresApproval: true,
    reviewStatus: "needs_user",
    reviewReason: "destructive command",
  }), handlers);

  assert.deepEqual(reviews, [{ status: "reviewing", reason: "" }]);
  assert.deepEqual(required, [{
    toolCallId: "call-1",
    params: { command: "rm -rf /tmp/example" },
    reason: "destructive command",
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

test("dispatchWSMessage routes Agent Team lifecycle and member identity", () => {
  const calls: string[] = [];
  const handlers: WSHandlers = {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onTeamStart: (run) => calls.push(`start:${run.id}:${run.status}`),
    onTeamDone: (run) => calls.push(`done:${run.id}:${run.status}`),
    onSubagentStart: (data) => calls.push(`member:${data.teamRunId}:${data.memberRunId}`),
  };

  dispatchWSMessage(JSON.stringify({
    type: "team_start", sessionId: "s1", teamRunId: "team-1", requestId: "request-1",
    status: "running", maxMembers: 8, maxParallel: 4, startedAt: "2026-07-29T00:00:00Z",
  }), handlers);
  dispatchWSMessage(JSON.stringify({
    type: "subagent_start", sessionId: "s1", parentToolCallId: "tool-1", subagentRunId: "sub-1",
    teamRunId: "team-1", memberRunId: "member-1", title: "Research", task: "Inspect",
  }), handlers);
  dispatchWSMessage(JSON.stringify({
    type: "team_done", sessionId: "s1", teamRunId: "team-1", requestId: "request-1",
    status: "succeeded", maxMembers: 8, maxParallel: 4, startedAt: "2026-07-29T00:00:00Z", finishedAt: "2026-07-29T00:00:03Z",
  }), handlers);

  assert.deepEqual(calls, ["start:team-1:running", "member:team-1:member-1", "done:team-1:succeeded"]);
});

test("dispatchWSMessage routes queued Agent Team members", () => {
  const calls: string[] = [];
  dispatchWSMessage(JSON.stringify({
    type: "team_member_queued", sessionId: "s1", teamRunId: "team-1", memberRunId: "member-1",
    toolCallId: "tool-1", title: "Research", task: "Inspect", status: "queued", createdAt: "2026-07-29T00:00:01Z",
  }), {
    onSession: () => {},
    onStart: () => {},
    onChunk: () => {},
    onDone: () => {},
    onError: () => {},
    onTeamMemberQueued: (member) => calls.push(`${member.id}:${member.teamRunId}:${member.status}`),
  });
  assert.deepEqual(calls, ["member-1:team-1:queued"]);
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

test("dispatchWSMessage preserves extended context usage layers, budget, and cooldown state", () => {
  const received: Array<{
    state?: string;
    layersCount: number;
    outputBudgetTokens?: number;
    outputBudgetUsed?: number;
  }> = [];

  dispatchWSMessage(
    JSON.stringify({
      type: "context_usage",
      sessionId: "sid-ext",
      modelConfigId: "model-1",
      usedTokens: 60000,
      totalTokens: 128000,
      usedPercent: 47,
      availablePercent: 53,
      isCompacted: false,
      state: "cooldown",
      outputBudgetTokens: 100000,
      outputBudgetUsed: 82000,
      layers: [
        { layer: "system", tokens: 4000 },
        { layer: "history", tokens: 56000 },
      ],
    }),
    {
      onSession: () => {},
      onStart: () => {},
      onChunk: () => {},
      onDone: () => {},
      onError: () => {},
      onContextUsage: (usage) => {
        received.push({
          state: usage.state,
          layersCount: usage.layers?.length ?? 0,
          outputBudgetTokens: usage.outputBudgetTokens,
          outputBudgetUsed: usage.outputBudgetUsed,
        });
      },
    },
  );

  assert.deepEqual(received, [{
    state: "cooldown",
    layersCount: 2,
    outputBudgetTokens: 100000,
    outputBudgetUsed: 82000,
  }]);
});

test("dispatchWSMessage routes done skillLearning metadata", () => {
  const received: Array<{ skillId?: string; status: string; message: string }> = [];
  const skillLearning = {
    skillId: "deploy-helper",
    status: "completed",
    message: "Saved release verification steps",
  };
  dispatchWSMessage(JSON.stringify({ type: "done", sessionId: "sid-skill", skillLearning }), {
    onStart: () => {},
    onDone: (_sid, _answer, meta) => {
      if (meta?.skillLearning) received.push(meta.skillLearning);
    },
  });
  assert.deepEqual(received, [skillLearning]);
});
