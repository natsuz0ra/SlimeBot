import assert from "node:assert/strict";
import test from "node:test";
import { createInitialState, reducer } from "./reducer.ts";

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
