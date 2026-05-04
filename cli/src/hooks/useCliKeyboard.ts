import { useInput } from "ink";
import type { Key } from "ink";
import type React from "react";
import type { AppAction, AppState, MCPTemplate, MenuItem, ModelProvider } from "../types.js";
import { adjustContextSize, clampContextSize } from "../utils/contextSize.js";
import { MCP_TEMPLATES } from "../types.js";
import type { CLISocket } from "../ws/socket.js";

export type ApprovalKeyAction =
  | { kind: "nav"; delta: number }
  | { kind: "mark"; toolCallId: string }
  | { kind: "settle"; items: Array<{ toolCallId: string; approved: boolean }> };

interface UseCliKeyboardProps {
  state: AppState;
  dispatch: React.Dispatch<AppAction>;
  socketRef: React.MutableRefObject<CLISocket | null>;
  exit: () => void;
  handleMenuSelect: (item: MenuItem | undefined) => Promise<void>;
  handleMenuDelete: (item: MenuItem | undefined) => Promise<void>;
  handleMenuAdd: () => void;
  handleMenuEdit: (item: MenuItem | undefined) => void;
  handleMenuToggle: (item: MenuItem | undefined) => Promise<void>;
  loadMCPConfigs: () => Promise<void>;
  loadModels: () => Promise<void>;
  saveMCPConfig: () => Promise<void>;
  saveModelConfig: () => Promise<void>;
  selectMCPTemplate: (template: MCPTemplate) => void;
  moveModelProvider: (current: ModelProvider, delta: number) => ModelProvider;
}

function cancelAnswers(state: AppState): string {
  return JSON.stringify(
    state.qaQuestions.map((q) => ({ questionId: q.id, selectedOption: -2, customAnswer: "" })),
  );
}

export function shouldLetQuestionAnswerViewHandleInput(state: AppState, input: string, key: Key): boolean {
  if (state.view !== "question-answer" || state.qaStep !== "questions") {
    return false;
  }
  const q = state.qaQuestions[state.qaCurrentIndex];
  if (!q) {
    return false;
  }
  const isCustomCursor = state.qaCursor === q.options.length;
  const isCustomSelected = state.qaAnswers[state.qaCurrentIndex]?.selectedOption === -1;
  if (!isCustomCursor && !isCustomSelected) {
    return false;
  }
  if (key.return || key.escape || key.tab || key.upArrow || key.downArrow) {
    return false;
  }
  if (key.ctrl || key.meta || key.leftArrow || key.rightArrow || key.pageUp || key.pageDown || key.home || key.end) {
    return false;
  }
  if (key.backspace || key.delete) {
    return true;
  }
  return Boolean(input);
}

export function getModelEditorFieldNavigationAction(state: AppState, key: Key): AppAction | null {
  if (state.view !== "model-editor" || state.modelEditorProviderSelect || !key.tab) {
    return null;
  }
  return key.shift ? { type: "MODEL_EDITOR_PREV_FIELD" } : { type: "MODEL_EDITOR_NEXT_FIELD" };
}

export function getQuestionAnswerConfirmEnterAction(state: AppState): AppAction | "submit" | null {
  if (state.view !== "question-answer" || state.qaStep !== "confirm") {
    return null;
  }
  return state.qaCursor < state.qaQuestions.length
    ? { type: "QA_EDIT_QUESTION", index: state.qaCursor }
    : "submit";
}

function getQuestionAnswerAdvanceAction(state: AppState): AppAction {
  return state.qaCurrentIndex < state.qaQuestions.length - 1
    ? { type: "QA_NEXT_QUESTION" }
    : { type: "QA_STEP_CONFIRM" };
}

export function getQuestionAnswerQuestionKeyActions(state: AppState, input: string, key: Key): AppAction[] | null {
  if (state.view !== "question-answer" || state.qaStep !== "questions") {
    return null;
  }
  const q = state.qaQuestions[state.qaCurrentIndex];
  if (!q) {
    return null;
  }

  if (key.leftArrow) {
    return [{ type: "QA_PREV_QUESTION" }];
  }
  if (key.rightArrow || key.tab) {
    return [getQuestionAnswerAdvanceAction(state)];
  }
  if (input === "c" || input === "C") {
    return [
      { type: "QA_NAV_TO", cursor: q.options.length },
      { type: "QA_SELECT", optionIndex: -1 },
    ];
  }
  if (/^[1-5]$/.test(input)) {
    const optionIndex = Number(input) - 1;
    if (optionIndex < q.options.length) {
      return [
        { type: "QA_SELECT", optionIndex },
        getQuestionAnswerAdvanceAction(state),
      ];
    }
  }
  return null;
}

function currentApprovalId(state: AppState): string {
  return (state.pendingApprovals[state.approvalCursor]?.toolCallId || state.approvalToolCallId || "").trim();
}

function approvalBatchItems(state: AppState, approved: boolean): Array<{ toolCallId: string; approved: boolean }> {
  return state.pendingApprovals
    .map((item) => ({ toolCallId: item.toolCallId, approved }))
    .filter((item) => item.toolCallId);
}

export function getApprovalKeyAction(state: AppState, input: string, key: Key): ApprovalKeyAction | null {
  if (state.view !== "approval") {
    return null;
  }
  if (key.upArrow) {
    return { kind: "nav", delta: -1 };
  }
  if (key.downArrow) {
    return { kind: "nav", delta: 1 };
  }

  const toolCallId = currentApprovalId(state);
  if (input === "y" || input === "Y") {
    return toolCallId ? { kind: "settle", items: [{ toolCallId, approved: true }] } : null;
  }
  if (input === "n" || input === "N") {
    return toolCallId ? { kind: "settle", items: [{ toolCallId, approved: false }] } : null;
  }
  if (input === "a" || input === "A") {
    const items = approvalBatchItems(state, true);
    return items.length > 0 ? { kind: "settle", items } : null;
  }
  if (input === "r" || input === "R") {
    const items = approvalBatchItems(state, false);
    return items.length > 0 ? { kind: "settle", items } : null;
  }
  return null;
}

export function useCliKeyboard({
  state,
  dispatch,
  socketRef,
  exit,
  handleMenuSelect,
  handleMenuDelete,
  handleMenuAdd,
  handleMenuEdit,
  handleMenuToggle,
  loadMCPConfigs,
  loadModels,
  saveMCPConfig,
  saveModelConfig,
  selectMCPTemplate,
  moveModelProvider,
}: UseCliKeyboardProps): void {
  useInput((input, key) => {
    if (key.ctrl && input === "c") {
      if (state.streaming) {
        const sent = state.sessionId && socketRef.current?.sendStop(state.sessionId) || false;
        if (!sent) {
          dispatch({ type: "STREAM_DONE", error: "Generation stopped (disconnected)." });
        }
        return;
      }
      socketRef.current?.close();
      exit();
      return;
    }

    if (key.tab && key.shift && state.view === "chat") {
      dispatch({ type: "TOGGLE_PLAN_MODE" });
      return;
    }

    if (state.view === "approval") {
      const settleApproval = (toolCallId: string, approved: boolean) => {
        socketRef.current?.sendToolApproval(toolCallId, approved);
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId,
            status: approved ? "executing" : "rejected",
            error: approved ? "" : "Execution was rejected by the user.",
            content: "",
          },
        });
        dispatch({ type: "REMOVE_PENDING_APPROVAL", toolCallId, approved });
      };
      const action = getApprovalKeyAction(state, input, key);
      if (action?.kind === "nav") {
        dispatch({ type: "APPROVAL_NAV", delta: action.delta });
      } else if (action?.kind === "mark") {
        dispatch({ type: "TOGGLE_APPROVAL_MARK", toolCallId: action.toolCallId });
      } else if (action?.kind === "settle") {
        for (const item of action.items) {
          settleApproval(item.toolCallId, item.approved);
        }
      }
      return;
    }

    if (state.view === "plan-confirm") {
      if (state.planConfirmCursor === 1) {
        if (key.upArrow) {
          dispatch({ type: "PLAN_CONFIRM_NAV", delta: -1 });
        }
        return;
      }
      if (key.upArrow) {
        dispatch({ type: "PLAN_CONFIRM_NAV", delta: -1 });
        return;
      }
      if (key.downArrow) {
        dispatch({ type: "PLAN_CONFIRM_NAV", delta: 1 });
        return;
      }
      if (key.return) {
        const displayContent = "Execute this plan";
        dispatch({ type: "APPEND_ENTRY", entry: { kind: "user", content: displayContent } });
        socketRef.current?.sendPlanApprove(
          state.pendingPlanId, state.sessionId, state.modelId, displayContent,
        );
        if (state.planMode) {
          dispatch({ type: "TOGGLE_PLAN_MODE" });
        }
        dispatch({ type: "CLEAR_PLAN_CONFIRMATION" });
        return;
      }
      if (key.escape) {
        socketRef.current?.sendPlanReject(
          state.pendingPlanId, state.sessionId,
        );
        dispatch({ type: "CLEAR_PLAN_CONFIRMATION" });
      }
      return;
    }

    if (state.view === "question-answer") {
      if (shouldLetQuestionAnswerViewHandleInput(state, input, key)) {
        return;
      }
      if (state.qaStep === "questions") {
        const questionActions = getQuestionAnswerQuestionKeyActions(state, input, key);
        if (questionActions) {
          for (const action of questionActions) {
            dispatch(action);
          }
          return;
        }
        if (key.upArrow) {
          dispatch({ type: "QA_NAV", delta: -1 });
          return;
        }
        if (key.downArrow) {
          dispatch({ type: "QA_NAV", delta: 1 });
          return;
        }
        if (key.return) {
          const q = state.qaQuestions[state.qaCurrentIndex];
          if (!q) return;
          const maxIdx = q.options.length;
          dispatch({ type: "QA_SELECT", optionIndex: state.qaCursor < maxIdx ? state.qaCursor : -1 });
          if (state.qaCursor < maxIdx) {
            dispatch(state.qaCurrentIndex < state.qaQuestions.length - 1
              ? { type: "QA_NEXT_QUESTION" }
              : { type: "QA_STEP_CONFIRM" });
          }
          return;
        }
        if (key.tab) {
          dispatch(getQuestionAnswerAdvanceAction(state));
          return;
        }
      } else {
        if (key.upArrow || key.downArrow) {
          dispatch({ type: "QA_CONFIRM_NAV", delta: key.upArrow ? -1 : 1 });
          return;
        }
        if (key.return) {
          const action = getQuestionAnswerConfirmEnterAction(state);
          if (action === "submit") {
            socketRef.current?.sendToolApproval(state.qaToolCallId, true, JSON.stringify(state.qaAnswers));
            dispatch({ type: "CLEAR_QA" });
          } else if (action) {
            dispatch(action);
          }
          return;
        }
        if (key.escape) {
          dispatch({ type: "QA_STEP_BACK" });
          return;
        }
      }
      if (key.escape) {
        socketRef.current?.sendToolApproval(state.qaToolCallId, true, cancelAnswers(state));
        dispatch({ type: "CLEAR_QA" });
      }
      return;
    }

    if (state.view === "thinking-detail") {
      if (key.escape) {
        dispatch({ type: "SET_VIEW", view: "chat" });
      }
      return;
    }

    if (state.view === "menu") {
      const current = state.menuItems[state.menuCursor];
      if (key.upArrow) {
        dispatch({ type: "MENU_NAV", delta: -1 });
        return;
      }
      if (key.downArrow) {
        dispatch({ type: "MENU_NAV", delta: 1 });
        return;
      }
      if (key.return) {
        void handleMenuSelect(current);
        return;
      }
      if (key.escape) {
        dispatch({ type: "CLOSE_MENU" });
        return;
      }
      if (input === "d") {
        void handleMenuDelete(current);
        return;
      }
      if (input === "a") {
        handleMenuAdd();
        return;
      }
      if (input === "e") {
        handleMenuEdit(current);
        return;
      }
      if (input === " ") {
        void handleMenuToggle(current);
      }
      return;
    }

    if (state.view === "mcp-editor") {
      if (key.tab) {
        dispatch({ type: "TOGGLE_MCP_EDITOR_FOCUS" });
        return;
      }
      if (key.escape) {
        void loadMCPConfigs();
        return;
      }
      if (key.ctrl && input === "e") {
        dispatch({ type: "TOGGLE_MCP_EDITOR_ENABLED" });
        return;
      }
      if (key.ctrl && input === "s") {
        void saveMCPConfig();
      }
      return;
    }

    if (state.view === "mcp-template") {
      if (key.upArrow) {
        dispatch({ type: "MCP_TEMPLATE_NAV", delta: -1 });
        return;
      }
      if (key.downArrow) {
        dispatch({ type: "MCP_TEMPLATE_NAV", delta: 1 });
        return;
      }
      if (key.return) {
        selectMCPTemplate(MCP_TEMPLATES[state.mcpTemplateCursor]);
        return;
      }
      if (key.escape) {
        void loadMCPConfigs();
      }
      return;
    }

    if (state.view === "model-editor") {
      if (state.modelEditorProviderSelect) {
        if (key.upArrow) {
          dispatch({ type: "SET_MODEL_EDITOR_PROVIDER", provider: moveModelProvider(state.modelEditorProvider, -1) });
          return;
        }
        if (key.downArrow) {
          dispatch({ type: "SET_MODEL_EDITOR_PROVIDER", provider: moveModelProvider(state.modelEditorProvider, 1) });
          return;
        }
        if (key.return || key.escape) {
          dispatch({ type: "TOGGLE_MODEL_EDITOR_PROVIDER_SELECT" });
          return;
        }
        return;
      }
      const fieldNavigationAction = getModelEditorFieldNavigationAction(state, key);
      if (fieldNavigationAction) {
        dispatch(fieldNavigationAction);
        return;
      }
      if (state.modelEditorFocusIndex === 5) {
        if (key.leftArrow || key.rightArrow || key.upArrow || key.downArrow) {
          const current = clampContextSize(state.modelEditorContextSize);
          const delta = key.leftArrow ? -1_000 : key.rightArrow ? 1_000 : key.upArrow ? 32_000 : -32_000;
          dispatch({ type: "SET_MODEL_EDITOR_CONTEXT_SIZE", contextSize: String(adjustContextSize(current, delta)) });
          return;
        }
      }
      if (key.escape) {
        void loadModels();
        return;
      }
      if (key.ctrl && input === "s") {
        void saveModelConfig();
        return;
      }
      if (key.return && state.modelEditorFocusIndex === 1) {
        dispatch({ type: "TOGGLE_MODEL_EDITOR_PROVIDER_SELECT" });
        return;
      }
    }

    if (state.view !== "chat") return;

    if (state.streaming && key.escape) {
      const sent = state.sessionId && socketRef.current?.sendStop(state.sessionId) || false;
      if (!sent) {
        dispatch({ type: "STREAM_DONE", error: "Generation stopped (disconnected)." });
      }
    }
  });
}
