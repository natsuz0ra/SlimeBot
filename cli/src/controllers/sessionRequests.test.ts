import assert from "node:assert/strict";
import test from "node:test";
import { applySessionResponse, captureSessionRequest } from "./sessionRequests.js";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

test("late history or errors from A cannot replace B's history", async () => {
  const selection = { current: { id: "a", name: "A", generation: 1 } };
  const aHistory = deferred<string>();
  const aFailure = deferred<string>();
  const bHistory = deferred<string>();
  const errors: unknown[] = [];
  let visibleHistory = "";
  const loadHistory = (history: Promise<string>) =>
    applySessionResponse(
      captureSessionRequest(selection),
      () => history,
      (value) => { visibleHistory = value; },
      (error) => errors.push(error),
    );

  const oldHistory = loadHistory(aHistory.promise);
  const oldError = loadHistory(aFailure.promise);
  selection.current = { id: "b", name: "B", generation: 2 };
  const newHistory = loadHistory(bHistory.promise);
  bHistory.resolve("B history");
  await newHistory;
  assert.equal(visibleHistory, "B history");

  aHistory.resolve("stale A history");
  aFailure.reject(new Error("stale A error"));
  await Promise.all([oldHistory, oldError]);
  assert.equal(visibleHistory, "B history");
  assert.deepEqual(errors, []);
});

test("context usage started before leaving A is stale after reopening A", async () => {
  const selection = { current: { id: "a", name: "A", generation: 1 } };
  const oldUsage = deferred<number>();
  const writes: number[] = [];
  const oldRequest = applySessionResponse(
    captureSessionRequest(selection),
    () => oldUsage.promise,
    (usage) => writes.push(usage),
  );

  selection.current = { id: "b", name: "B", generation: 2 };
  selection.current = { id: "a", name: "A", generation: 3 };
  await applySessionResponse(captureSessionRequest(selection), async () => 200, (usage) => writes.push(usage));
  oldUsage.resolve(100);
  await oldRequest;
  assert.deepEqual(writes, [200]);
});
