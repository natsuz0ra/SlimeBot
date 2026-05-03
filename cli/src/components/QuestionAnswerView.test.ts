import assert from "node:assert/strict";
import test from "node:test";
import {
  getQuestionAnswerProgress,
  getQuestionAnswerTitle,
  getQuestionAnswerViewHints,
} from "./QuestionAnswerView.js";

test("question answer title names the wizard and current position", () => {
  assert.equal(getQuestionAnswerTitle(0, 3), "Answer questions  Question 1 of 3");
});

test("question answer progress uses filled and empty dots", () => {
  assert.deepEqual(getQuestionAnswerProgress(1, 4), ["●", "●", "○", "○"]);
});

test("question answer hints include custom and review actions", () => {
  assert.equal(
    getQuestionAnswerViewHints("questions"),
    "Enter select & next | ←/→ prev/next | C custom | Esc cancel",
  );
  assert.equal(
    getQuestionAnswerViewHints("confirm"),
    "↑/↓ select | Enter edit/send | Esc back to questions",
  );
});
