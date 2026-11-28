import assert from "node:assert/strict";
import test from "node:test";

import { createInitialState, reducer } from "../reducer.js";
import type { AppAction, UpdateCheckResult, UpdateJobStatus } from "../types.js";
import { createUpdateController, type UpdateControllerApi } from "./updateController.js";

const check: UpdateCheckResult = {
  current: "1.36.0", latest: "1.37.0", updateAvailable: true, canApply: true,
};
const downloading: UpdateJobStatus = { phase: "downloading", target: "1.37.0", progressPercent: 25 };

test("update controller performs a silent startup check and applies an available update", async () => {
  let state = createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
  const forced: boolean[] = [];
  const applied: string[] = [];
  const messages: string[] = [];
  const api: UpdateControllerApi = {
    getUpdateCheck: async (force = false) => { forced.push(force); return check; },
    getUpdateJob: async () => downloading,
    applyUpdate: async (version = "") => { applied.push(version); return downloading; },
  };
  const controller = createUpdateController({
    api,
    dispatch: (action: AppAction) => { state = reducer(state, action); },
    appendSystem: message => messages.push(message),
    getState: () => state,
  });

  await controller.checkForUpdatesSilently();
  assert.deepEqual(forced, [false]);
  assert.equal(state.updateCheck?.latest, "1.37.0");

  await controller.applyUpdate();
  assert.deepEqual(applied, ["1.37.0"]);
  assert.equal(state.updateApplying, true);
  assert.equal(state.updateJob?.phase, "downloading");
  assert.match(messages.at(-1) || "", /Update started for 1.37.0/);
});

test("update controller ignores a polling result cancelled during the request", async () => {
  let state = createInitialState("http://localhost", "token", "C:/repo", "1.37.0");
  let finish!: (job: UpdateJobStatus) => void;
  const controller = createUpdateController({
    api: {
      getUpdateCheck: async () => check,
      getUpdateJob: () => new Promise(resolve => { finish = resolve; }),
      applyUpdate: async () => downloading,
    },
    dispatch: (action: AppAction) => { state = reducer(state, action); },
    appendSystem: () => {},
    getState: () => state,
  });

  let cancelled = false;
  const polling = controller.refreshUpdateJob(() => cancelled);
  cancelled = true;
  finish(downloading);
  await polling;
  assert.equal(state.updateJob, null);
  assert.equal(state.updateApplying, false);
});
