import assert from "node:assert/strict";
import test from "node:test";
import type { Key } from "ink";
import { createInitialState } from "../reducer.js";
import {
  getApprovalKeyAction,
  getModelEditorFieldNavigationAction,
  getQuestionAnswerConfirmEnterAction,
  getQuestionAnswerQuestionKeyActions,
  shouldLetQuestionAnswerViewHandleInput,
} from "./useCliKeyboard.js";

function key(overrides: Partial<Key> = {}): Key {
  return {
    upArrow: false,
    downArrow: false,
    leftArrow: false,
    rightArrow: false,
    pageDown: false,
    pageUp: false,
    home: false,
    end: false,
    return: false,
    escape: false,
    ctrl: false,
    shift: false,
    tab: false,
    backspace: false,
    delete: false,
    meta: false,
    ...overrides,
  } as Key;
}

test("custom input cursor lets question view handle printable input", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "questions" as const,
    qaCurrentIndex: 0,
    qaCursor: 1,
    qaQuestions: [{ id: "q1", question: "Q", options: ["A"] }],
    qaAnswers: [{ questionId: "q1", selectedOption: -1, customAnswer: "" }],
  };

  assert.equal(shouldLetQuestionAnswerViewHandleInput(state, "h", key()), true);
  assert.equal(shouldLetQuestionAnswerViewHandleInput(state, "", key({ backspace: true })), true);
});

test("navigation keys are still owned by global keyboard handler", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "questions" as const,
    qaCurrentIndex: 0,
    qaCursor: 1,
    qaQuestions: [{ id: "q1", question: "Q", options: ["A"] }],
    qaAnswers: [{ questionId: "q1", selectedOption: -1, customAnswer: "" }],
  };

  assert.equal(shouldLetQuestionAnswerViewHandleInput(state, "", key({ upArrow: true })), false);
  assert.equal(shouldLetQuestionAnswerViewHandleInput(state, "", key({ tab: true })), false);
  assert.equal(shouldLetQuestionAnswerViewHandleInput(state, "", key({ escape: true })), false);
});

test("model editor tab navigation chooses forward and reverse actions", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "model-editor" as const,
    modelEditorProviderSelect: false,
  };

  assert.deepEqual(getModelEditorFieldNavigationAction(state, key({ tab: true })), { type: "MODEL_EDITOR_NEXT_FIELD" });
  assert.deepEqual(getModelEditorFieldNavigationAction(state, key({ tab: true, shift: true })), { type: "MODEL_EDITOR_PREV_FIELD" });
});

test("model editor tab navigation is ignored while provider select is open", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "model-editor" as const,
    modelEditorProviderSelect: true,
  };

  assert.equal(getModelEditorFieldNavigationAction(state, key({ tab: true })), null);
  assert.equal(getModelEditorFieldNavigationAction(state, key({ tab: true, shift: true })), null);
});

test("approval keyboard uses Y and N for the current item", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "approval" as const,
    approvalCursor: 1,
    pendingApprovals: [
      { toolCallId: "call-a", toolName: "exec", command: "run", params: {} },
      { toolCallId: "call-b", toolName: "file_read", command: "read", params: {} },
    ],
  };

  assert.deepEqual(getApprovalKeyAction(state, "Y", key()), {
    kind: "settle",
    items: [{ toolCallId: "call-b", approved: true }],
  });
  assert.deepEqual(getApprovalKeyAction(state, "N", key()), {
    kind: "settle",
    items: [{ toolCallId: "call-b", approved: false }],
  });
  assert.equal(getApprovalKeyAction(state, "", key({ return: true })), null);
  assert.equal(getApprovalKeyAction(state, " ", key()), null);
});

test("approval keyboard batches every pending item regardless of marks", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "approval" as const,
    approvalCursor: 0,
    markedApprovalIds: ["call-b"],
    pendingApprovals: [
      { toolCallId: "call-a", toolName: "exec", command: "run", params: {} },
      { toolCallId: "call-b", toolName: "file_read", command: "read", params: {} },
      { toolCallId: "call-c", toolName: "file_edit", command: "edit", params: {} },
    ],
  };

  assert.deepEqual(getApprovalKeyAction(state, "A", key()), {
    kind: "settle",
    items: [
      { toolCallId: "call-a", approved: true },
      { toolCallId: "call-b", approved: true },
      { toolCallId: "call-c", approved: true },
    ],
  });
  assert.deepEqual(getApprovalKeyAction(state, "R", key()), {
    kind: "settle",
    items: [
      { toolCallId: "call-a", approved: false },
      { toolCallId: "call-b", approved: false },
      { toolCallId: "call-c", approved: false },
    ],
  });
});

test("approval keyboard accepts lowercase shortcuts", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "approval" as const,
    approvalCursor: 0,
    pendingApprovals: [
      { toolCallId: "call-a", toolName: "exec", command: "run", params: {} },
      { toolCallId: "call-b", toolName: "file_read", command: "read", params: {} },
    ],
  };

  assert.deepEqual(getApprovalKeyAction(state, "y", key()), {
    kind: "settle",
    items: [{ toolCallId: "call-a", approved: true }],
  });
  assert.deepEqual(getApprovalKeyAction(state, "n", key()), {
    kind: "settle",
    items: [{ toolCallId: "call-a", approved: false }],
  });
});

test("question answer confirm enter edits selected answer before submit row", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "confirm" as const,
    qaCursor: 1,
    qaQuestions: [
      { id: "q1", question: "Q1", options: ["A"] },
      { id: "q2", question: "Q2", options: ["B"] },
    ],
  };

  assert.deepEqual(getQuestionAnswerConfirmEnterAction(state), { type: "QA_EDIT_QUESTION", index: 1 });
});

test("question answer confirm enter submits on final row", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "confirm" as const,
    qaCursor: 2,
    qaQuestions: [
      { id: "q1", question: "Q1", options: ["A"] },
      { id: "q2", question: "Q2", options: ["B"] },
    ],
  };

  assert.equal(getQuestionAnswerConfirmEnterAction(state), "submit");
});

test("question answer number key selects matching preset answer and advances", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "questions" as const,
    qaCurrentIndex: 0,
    qaQuestions: [
      { id: "q1", question: "Q1", options: ["A", "B", "C"] },
      { id: "q2", question: "Q2", options: ["D"] },
    ],
  };

  assert.deepEqual(getQuestionAnswerQuestionKeyActions(state, "2", key()), [
    { type: "QA_SELECT", optionIndex: 1 },
    { type: "QA_NEXT_QUESTION" },
  ]);
});

test("question answer number key selects matching preset answer and opens confirm on last question", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "questions" as const,
    qaCurrentIndex: 0,
    qaQuestions: [{ id: "q1", question: "Q1", options: ["A", "B"] }],
  };

  assert.deepEqual(getQuestionAnswerQuestionKeyActions(state, "2", key()), [
    { type: "QA_SELECT", optionIndex: 1 },
    { type: "QA_STEP_CONFIRM" },
  ]);
});

test("question answer c key focuses custom answer row", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "questions" as const,
    qaCurrentIndex: 0,
    qaQuestions: [{ id: "q1", question: "Q1", options: ["A", "B"] }],
  };

  assert.deepEqual(getQuestionAnswerQuestionKeyActions(state, "C", key()), [
    { type: "QA_NAV_TO", cursor: 2 },
    { type: "QA_SELECT", optionIndex: -1 },
  ]);
});

test("question answer left and right arrows navigate between questions", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "question-answer" as const,
    qaStep: "questions" as const,
    qaCurrentIndex: 1,
    qaQuestions: [
      { id: "q1", question: "Q1", options: ["A"] },
      { id: "q2", question: "Q2", options: ["B"] },
    ],
  };

  assert.deepEqual(getQuestionAnswerQuestionKeyActions(state, "", key({ leftArrow: true })), [
    { type: "QA_PREV_QUESTION" },
  ]);
  assert.deepEqual(getQuestionAnswerQuestionKeyActions(state, "", key({ rightArrow: true })), [
    { type: "QA_STEP_CONFIRM" },
  ]);
});
