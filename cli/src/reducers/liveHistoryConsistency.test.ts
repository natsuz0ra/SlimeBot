import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { parseChatTimestamp } from "../../../shared/chat-protocol/timeline.js";
import { createInitialState, reducer } from "../reducer.js";
import type { AppAction, AppState, TimelineEntry } from "../types.js";
import { mapHistoryMessages } from "../utils/history.js";
import { dispatchWSMessage } from "../ws/socket.js";

const fixture = JSON.parse(readFileSync(new URL("../../../shared/chat-protocol/fixtures/turn-consistency.json", import.meta.url), "utf8"));

function reduceLiveEvents(): AppState {
  let state = createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
  const dispatch = (action: AppAction) => { state = reducer(state, action); };
  for (const event of fixture.events) {
    const result = dispatchWSMessage(JSON.stringify(event), {
      onStart: () => dispatch({ type: "STREAM_START", startedAt: Date.parse("2026-04-29T01:00:00Z") }),
      onChunk: chunk => dispatch({ type: "STREAM_CHUNK", chunk }),
      onThinkingStart: data => dispatch({
        type: "THINKING_START", startedAt: parseChatTimestamp(data.startedAt),
        parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId,
      }),
      onThinkingChunk: data => dispatch({
        type: "THINKING_CHUNK", chunk: data.content || "", startedAt: parseChatTimestamp(data.startedAt),
        parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId,
      }),
      onThinkingDone: data => dispatch({
        type: "THINKING_DONE", finishedAt: parseChatTimestamp(data.finishedAt),
        parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId,
      }),
      onToolCallStart: data => {
        dispatch({ type: "FLUSH_AND_WAIT" });
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool", content: "", toolCallId: data.toolCallId, toolName: data.toolName,
            command: data.command, params: data.params, status: data.requiresApproval ? "pending" : "executing",
            parentToolCallId: data.parentToolCallId, subagentRunId: data.subagentRunId,
          },
        });
      },
      onSubagentStart: data => dispatch({
        type: "UPSERT_TOOL_ENTRY",
        entry: {
          kind: "tool", content: "", toolCallId: data.parentToolCallId, toolName: "run_subagent",
          command: "delegate", status: "executing", subagentRunId: data.subagentRunId,
          subagentTitle: data.title, subagentTask: data.task,
        },
      }),
      onSubagentChunk: data => dispatch({
        type: "APPEND_SUBAGENT_STREAM", parentToolCallId: data.parentToolCallId, content: data.content,
      }),
      onSubagentDone: data => dispatch({
        type: "SUBAGENT_DONE", parentToolCallId: data.parentToolCallId, error: data.error,
        finishedAt: Date.parse("2026-04-29T01:00:08Z"),
      }),
      onToolCallResult: data => {
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool", content: data.output || data.error || "", toolCallId: data.toolCallId,
            toolName: data.toolName, command: data.command, status: data.status,
            output: data.output, error: data.error, metadata: data.metadata,
          },
        });
        dispatch({ type: "REMOVE_PENDING_APPROVAL", toolCallId: data.toolCallId });
      },
      onPlanStart: () => dispatch({ type: "PLAN_START" }),
      onPlanChunk: chunk => dispatch({ type: "PLAN_CHUNK", chunk }),
      onPlanBody: planBody => dispatch({ type: "PLAN_BODY", planBody }),
      onDone: () => dispatch({ type: "STREAM_DONE", error: null }),
    });
    assert.equal(result.ok, true);
  }
  return state;
}

function comparableTimeline(entries: TimelineEntry[]) {
  return entries.filter(entry => !entry.parentToolCallId).map(entry => {
    if (entry.kind === "tool") {
      return {
        kind: "tool",
        toolCallId: entry.toolCallId,
        status: entry.status,
        title: entry.subagentTitle,
        output: entry.output,
        thinkings: entry.subagentThinkings?.map(thinking => ({ content: thinking.content, done: thinking.thinkingDone })),
      };
    }
    if (entry.kind === "thinking") {
      return { kind: "thinking", content: entry.content, done: entry.thinkingDone, durationMs: entry.thinkingDurationMs };
    }
    return { kind: entry.kind === "assistant" ? "text" : entry.kind, content: entry.content };
  });
}

test("live WebSocket reduction matches persisted history projection", () => {
  const live = reduceLiveEvents();
  const message = {
    id: "assistant-1", sessionId: fixture.sessionId, role: "assistant" as const,
    content: fixture.history.content, seq: 1, createdAt: "2026-04-29T01:00:00Z",
  };
  const replayed = mapHistoryMessages(
    [message],
    { [message.id]: fixture.history.toolCalls },
    { [message.id]: fixture.history.thinking },
  );

  assert.deepEqual(comparableTimeline(live.timeline), comparableTimeline(replayed));
  assert.deepEqual(comparableTimeline(live.timeline).map(entry => entry.kind), [
    "thinking", "text", "tool", "plan", "text",
  ]);
  const parent = live.timeline.find(entry => entry.kind === "tool" && entry.toolCallId === "parent-tool");
  assert.equal(parent?.status, "completed");
  assert.equal(parent?.subagentThinkings?.length, 2);
  assert.equal(parent?.subagentThinkings?.every(thinking => thinking.thinkingDone), true);
});
