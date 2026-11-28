import assert from "node:assert/strict";
import test from "node:test";

import { createInitialState, reducer } from "../reducer.js";
import { chatReducer } from "./chatReducer.js";
import { consoleReducer } from "./consoleReducer.js";
import { navigationReducer } from "./navigationReducer.js";
import { settingsReducer } from "./settingsReducer.js";

function initialState() {
  return createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
}

test("feature slices decline actions owned by another domain", () => {
  const state = initialState();
  assert.equal(navigationReducer(state, { type: "STREAM_START" }), null);
  assert.equal(settingsReducer(state, { type: "SET_MENU", kind: "help", title: "Help", items: [], hint: "" }), null);
  assert.equal(consoleReducer(state, { type: "SET_MODEL", modelId: "model", modelName: "Model" }), null);
  assert.equal(chatReducer(state, { type: "SET_UPDATE_STATE", loading: true }), null);
});

test("root reducer composes navigation, settings, console, and chat without changing their behavior", () => {
  let state = reducer(initialState(), { type: "SET_MENU", kind: "help", title: "Help", items: [], hint: "" });
  assert.equal(state.view, "menu");

  state = reducer(state, { type: "SET_MODEL", modelId: "model-1", modelName: "Model" });
  assert.equal(state.modelId, "model-1");

  state = reducer(state, { type: "SET_UPDATE_STATE", loading: true });
  assert.equal(state.updateLoading, true);

  state = reducer(state, { type: "STREAM_START", startedAt: 1000 });
  assert.equal(state.streaming, true);
  assert.equal(state.turnStartedAt, 1000);
});

test("root reducer preserves state for an unknown action", () => {
  const state = initialState();
  assert.equal(reducer(state, { type: "FUTURE_ACTION" } as never), state);
});
