import assert from "node:assert/strict";
import test from "node:test";
import { completeCommand, matchCommandHints, moveCommandHintCursor } from "./commands.js";

test("matchCommandHints returns matching command prefixes", () => {
  assert.deepEqual(
    matchCommandHints("/m").map((hint) => hint.command),
    ["/model", "/mcp"],
  );
});

test("matchCommandHints ignores completed commands with trailing content", () => {
  assert.deepEqual(matchCommandHints("/model "), []);
  assert.deepEqual(matchCommandHints("/model abc"), []);
});

test("completeCommand fills the selected matching command", () => {
  assert.equal(completeCommand("/m", 1), "/mcp");
});

test("completeCommand safely clamps out-of-range selected indexes", () => {
  assert.equal(completeCommand("/m", 99), "/mcp");
  assert.equal(completeCommand("/m", -99), "/model");
});

test("moveCommandHintCursor wraps through command hints", () => {
  assert.equal(moveCommandHintCursor(0, -1, 2), 1);
  assert.equal(moveCommandHintCursor(1, 1, 2), 0);
  assert.equal(moveCommandHintCursor(0, 1, 0), 0);
});
