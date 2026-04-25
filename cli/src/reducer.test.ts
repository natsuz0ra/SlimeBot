import assert from "node:assert/strict";
import test from "node:test";
import { createInitialState, reducer } from "./reducer";

function initState() {
  return createInitialState("http://127.0.0.1:8080", "token", "C:/repo", "1.0.0");
}

test("createInitialState initializes empty input value", () => {
  const state = initState();
  assert.equal(state.inputValue, "");
  assert.equal(state.sessionName, "");
});

test("SET_INPUT updates value", () => {
  const state = reducer(
    initState(),
    { type: "SET_INPUT", value: "hello" } as any,
  );

  assert.equal(state.inputValue, "hello");
});

test("RESET_SESSION keeps input value and clears session state", () => {
  let state = reducer(
    initState(),
    { type: "SET_INPUT", value: "cursor" } as any,
  );
  state = reducer(state, {
    type: "SET_SESSION",
    sessionId: "sid-1",
    sessionName: "My Session",
  } as any);
  state = reducer(state, { type: "APPEND_ENTRY", entry: { kind: "system", content: "x" } });

  state = reducer(state, { type: "RESET_SESSION" });
  assert.equal(state.inputValue, "cursor");
  assert.equal(state.sessionId, "");
  assert.equal(state.sessionName, "");
  assert.equal(state.timeline.length, 0);
});

test("SET_SESSION_NAME updates current session title", () => {
  let state = reducer(
    initState(),
    { type: "SET_SESSION", sessionId: "sid-2", sessionName: "First Name" } as any,
  );
  state = reducer(state, { type: "SET_SESSION_NAME", sessionName: "Renamed" } as any);
  assert.equal(state.sessionName, "Renamed");
});

test("TOGGLE_COMPACT switches compact mode on and off", () => {
  let state = initState();
  assert.equal(state.compact, true);

  state = reducer(state, { type: "TOGGLE_COMPACT" } as any);
  assert.equal(state.compact, false);

  state = reducer(state, { type: "TOGGLE_COMPACT" } as any);
  assert.equal(state.compact, true);
});

test("TOGGLE_TOOL_OUTPUT switches tool output expanded on and off", () => {
  let state = initState();
  assert.equal(state.toolOutputExpanded, false);

  state = reducer(state, { type: "TOGGLE_TOOL_OUTPUT" } as any);
  assert.equal(state.toolOutputExpanded, true);

  state = reducer(state, { type: "TOGGLE_TOOL_OUTPUT" } as any);
  assert.equal(state.toolOutputExpanded, false);
});

test("THINKING_DONE stores a fixed thinking duration", () => {
  const state = {
    ...initState(),
    timeline: [
      {
        kind: "thinking",
        content: "reasoning",
        thinkingDone: false,
        thinkingStartedAt: 1_000,
      },
    ],
  };

  const next = reducer(state, { type: "THINKING_DONE", finishedAt: 2_750 } as any);
  const entry = next.timeline[0];

  assert.equal(entry.kind, "thinking");
  assert.equal(entry.thinkingDone, true);
  assert.equal(entry.thinkingDurationMs, 1_750);
});

test("THINKING_START flushes live assistant text before new thinking block", () => {
  const state = {
    ...initState(),
    liveAssistant: "spoken before more thought",
  };

  const next = reducer(state, { type: "THINKING_START" } as any);

  assert.deepEqual(next.timeline.map((entry) => entry.kind), ["assistant", "thinking"]);
  assert.equal(next.timeline[0].content, "spoken before more thought");
  assert.equal(next.liveAssistant, "");
});

test("STREAM_DONE keeps final assistant when previous timeline already has a plan", () => {
  const state = {
    ...initState(),
    timeline: [{ kind: "plan", content: "# Existing plan" }],
    liveAssistant: "execution finished",
    streaming: true,
  };

  const next = reducer(state, { type: "STREAM_DONE", error: null } as any);

  assert.deepEqual(next.timeline.map((entry) => entry.kind), ["plan", "assistant"]);
  assert.equal(next.timeline[1].content, "execution finished");
});

test("STREAM_DONE does not duplicate a plan body received in the current turn", () => {
  let state = {
    ...initState(),
    liveAssistant: "# Plan",
    streaming: true,
  };

  state = reducer(state, { type: "PLAN_BODY", planBody: "# Plan" } as any);
  state = reducer(state, { type: "STREAM_DONE", error: null } as any);

  assert.deepEqual(state.timeline.map((entry) => entry.kind), ["assistant", "plan"]);
  assert.equal(state.timeline.filter((entry) => entry.kind === "plan").length, 1);
});
