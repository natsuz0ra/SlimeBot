import assert from "node:assert/strict";
import test from "node:test";
import type { Key } from "ink";
import { createInitialState } from "../reducer.js";
import {
  getApprovalKeyAction,
  getTeamDetailKeyAction,
  getMenuNavigationAction,
  handleStreamingChatShortcut,
  getModelEditorFieldNavigationAction,
  getQuestionAnswerConfirmEnterAction,
  getQuestionAnswerQuestionKeyActions,
  getUpdateKeyAction,
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

test("team detail keyboard maps Escape and arrow navigation", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "team-detail" as const,
  };
  assert.deepEqual(getTeamDetailKeyAction(state, key({ escape: true })), { type: "SET_VIEW", view: "chat" });
  assert.deepEqual(getTeamDetailKeyAction(state, key({ leftArrow: true })), { type: "TEAM_DETAIL_NAV_TEAM", delta: -1 });
  assert.deepEqual(getTeamDetailKeyAction(state, key({ rightArrow: true })), { type: "TEAM_DETAIL_NAV_TEAM", delta: 1 });
  assert.deepEqual(getTeamDetailKeyAction(state, key({ upArrow: true })), { type: "TEAM_DETAIL_NAV_MEMBER", delta: -1 });
  assert.deepEqual(getTeamDetailKeyAction(state, key({ downArrow: true })), { type: "TEAM_DETAIL_NAV_MEMBER", delta: 1 });
  assert.equal(getTeamDetailKeyAction(state, key()), null);
});

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

test("streaming chat handles Ctrl+O and raw Ctrl+O globally", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "chat" as const,
    streaming: true,
  };
  const actions: string[] = [];
  const dispatch = (action: { type: string }) => {
    actions.push(action.type);
    return action as any;
  };

  assert.equal(handleStreamingChatShortcut(state, "o", key({ ctrl: true }), dispatch as any), true);
  assert.equal(handleStreamingChatShortcut(state, String.fromCharCode(15), key(), dispatch as any), true);
  assert.equal(actions.filter((action) => action === "TOGGLE_TOOL_OUTPUT").length, 2);
});

test("streaming chat shortcut does not grab Ctrl+C stop handling", () => {
  const state = {
    ...createInitialState("http://127.0.0.1:8080", "token", "/tmp", "1.0.0"),
    view: "chat" as const,
    streaming: true,
  };
  const actions: string[] = [];
  const dispatch = (action: { type: string }) => {
    actions.push(action.type);
    return action as any;
  };

  assert.equal(handleStreamingChatShortcut(state, "c", key({ ctrl: true }), dispatch as any), false);
  assert.deepEqual(actions, []);
});

test("update keyboard asks for confirmation before applying", () => {
  assert.deepEqual(getUpdateKeyAction("u", key(), false), { kind: "confirm" });
  assert.deepEqual(getUpdateKeyAction("Y", key(), true), { kind: "apply" });
  assert.deepEqual(getUpdateKeyAction("n", key(), true), { kind: "cancelConfirm" });
});

test("update keyboard escape cancels confirmation before returning", () => {
  assert.deepEqual(getUpdateKeyAction("", key({ escape: true }), true), { kind: "cancelConfirm" });
  assert.deepEqual(getUpdateKeyAction("", key({ escape: true }), false), { kind: "return" });
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

test("cooperative view owns navigation and Escape instead of mutating legacy team cursors",()=>{
 const state={...createInitialState("http://127.0.0.1:8080","token","/tmp","1.34.0"),view:"team-detail" as const};
 for(const input of [key({escape:true}),key({leftArrow:true}),key({rightArrow:true}),key({upArrow:true}),key({downArrow:true})]) assert.equal(getTeamDetailKeyAction(state,input,true),null);
});

test("menu navigation supports paging, boundaries and wheel scrolling", () => {
  assert.deepEqual(getMenuNavigationAction(key({ pageDown: true }), 30, 4), { type: "MENU_NAV", delta: 4 });
  assert.deepEqual(getMenuNavigationAction(key({ pageUp: true }), 30, 4), { type: "MENU_NAV", delta: -4 });
  assert.deepEqual(getMenuNavigationAction(key({ home: true }), 30, 4), { type: "MENU_NAV", delta: -30 });
  assert.deepEqual(getMenuNavigationAction(key({ end: true }), 30, 4), { type: "MENU_NAV", delta: 30 });
  assert.deepEqual(getMenuNavigationAction(key({ wheelDown: true }), 30, 4), { type: "MENU_NAV", delta: 1 });
  assert.deepEqual(getMenuNavigationAction(key({ pageDown: true }), 0, 0), { type: "MENU_NAV", delta: 1 });
  assert.equal(getMenuNavigationAction(key({ return: true }), 30, 4), null);
});
