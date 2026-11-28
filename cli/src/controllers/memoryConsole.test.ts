import assert from "node:assert/strict";
import test from "node:test";

import { createInitialState, reducer } from "../reducer.js";
import type { AppAction, MemorySnapshot, Settings } from "../types.js";
import { createMemoryConsoleController, type MemoryConsoleApi } from "./memoryConsole.js";

function snapshot(enabled = true): MemorySnapshot {
  return {
    memoryEnabled: enabled,
    memoryUserProfileEnabled: true,
    memoryNudgeInterval: 10,
    memoryDirectory: "/tmp/memory",
    memory: { target: "memory", entries: [], usageChars: 0, charLimit: 2200, entryCount: 0, enabled },
    user: { target: "user", entries: [], usageChars: 0, charLimit: 1375, entryCount: 0, enabled: true },
  };
}

function setup() {
  let state = createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
  let currentSnapshot = snapshot();
  const settingsPatches: Partial<Settings>[] = [];
  const api: MemoryConsoleApi = {
    getMemory: async () => currentSnapshot,
    clearMemory: async () => {},
    updateSettings: async settings => { settingsPatches.push(settings); },
  };
  const dispatch = (action: AppAction) => { state = reducer(state, action); };
  const controller = createMemoryConsoleController({
    api,
    dispatch,
    appendSystem: () => {},
    getState: () => state,
  });
  return {
    controller,
    dispatch,
    settingsPatches,
    getState: () => state,
    setSnapshot: (next: MemorySnapshot) => { currentSnapshot = next; },
  };
}

test("memory controller loads the dedicated console without printing a snapshot", async () => {
  const { controller, getState } = setup();
  await controller.loadMemory();

  assert.equal(getState().view, "memory-console");
  assert.equal(getState().memoryLoading, false);
  assert.equal(getState().memorySnapshot?.memory.charLimit, 2200);
});

test("memory controller toggles settings and saves a validated draft through the API", async () => {
  const { controller, dispatch, settingsPatches, getState, setSnapshot } = setup();
  await controller.loadMemory();
  setSnapshot(snapshot(false));
  await controller.handleMemoryConsoleSelect();
  assert.deepEqual(settingsPatches[0], { memoryEnabled: false });
  assert.equal(getState().memorySnapshot?.memoryEnabled, false);

  controller.startMemoryConsoleEdit("memoryCharLimit");
  dispatch({ type: "MEMORY_CONSOLE_SET_DRAFT", draft: "3000" });
  await controller.saveMemoryConsoleDraft();
  assert.deepEqual(settingsPatches[1], { memoryCharLimit: 3000 });
  assert.equal(getState().memoryMode, "actions");
});
