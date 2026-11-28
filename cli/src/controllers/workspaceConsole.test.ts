import assert from "node:assert/strict";
import test from "node:test";

import { createInitialState, reducer } from "../reducer.js";
import type { AppAction, Settings } from "../types.js";
import { createWorkspaceConsoleController, type WorkspaceConsoleApi } from "./workspaceConsole.js";

test("workspace console persists usage budgets and refreshes the active session", async () => {
  let state = createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
  const dispatch = (action: AppAction) => { state = reducer(state, action); };
  dispatch({ type: "SET_MODEL", modelId: "model-1", modelName: "Model" });
  const patches: Partial<Settings>[] = [];
  const refreshed: string[] = [];
  const messages: string[] = [];
  let budget = 0;
  const api: WorkspaceConsoleApi = {
    listCheckpoints: async () => [],
    updateSettings: async patch => { patches.push(patch); },
  };
  const controller = createWorkspaceConsoleController({
    api,
    dispatch,
    appendSystem: message => messages.push(message),
    getState: () => state,
    getSessionId: () => "session-1",
    getOutputBudgetTokens: () => budget,
    setOutputBudgetTokens: tokens => { budget = tokens; },
    refreshContextUsage: async (sessionId, modelId) => { refreshed.push(`${sessionId}:${modelId}`); },
  });

  await controller.handleUsageCommand("budget 2,500");
  assert.deepEqual(patches, [{ sessionOutputTokenBudget: 2500 }]);
  assert.equal(budget, 2500);
  assert.deepEqual(refreshed, ["session-1:model-1"]);
  assert.match(messages.at(-1) || "", /budget set to/);

  await controller.handleUsageCommand("");
  assert.equal(state.view, "usage");
});

test("workspace console lists checkpoints or opens the interactive checkpoint view", async () => {
  let state = createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
  const messages: string[] = [];
  const controller = createWorkspaceConsoleController({
    api: {
      listCheckpoints: async () => [{
        id: "checkpoint-123456", createdAt: "2026-01-01T00:00:00Z", kind: "turn", fileCount: 2,
      }],
      updateSettings: async () => {},
    },
    dispatch: (action: AppAction) => { state = reducer(state, action); },
    appendSystem: message => messages.push(message),
    getState: () => state,
    getSessionId: () => "session-1",
    getOutputBudgetTokens: () => 0,
    setOutputBudgetTokens: () => {},
    refreshContextUsage: async () => {},
  });

  await controller.handleCheckpointCommand("list");
  assert.match(messages[0] || "", /checkpoi · turn · 2 file\(s\)/);
  await controller.handleCheckpointCommand("");
  assert.equal(state.view, "checkpoint");
});
