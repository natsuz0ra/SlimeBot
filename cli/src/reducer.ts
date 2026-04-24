/**
 * App reducer — global state.
 * Immutable updates: always returns a new state object.
 */

import type {
  AppState,
  AppAction,
  TimelineEntry,
  MenuKind,
  ViewMode,
  ModelProvider,
} from "./types.js";

export function createInitialState(
  apiURL: string,
  cliToken: string,
  cwd: string,
  version: string,
): AppState {
  return {
    view: "chat",
    sessionId: "",
    sessionName: "",
    modelId: "",
    modelName: "(none)",
    thinkingLevel: "off",
    approvalMode: "standard",
    timeline: [],
    streaming: false,
    assistantWaiting: false,
    liveAssistant: "",
    blinkOn: true,
    compact: true,
    toolOutputExpanded: false,
    planMode: false,
    planGenerating: false,
    planReceived: false,
    thinkingDetailContent: "",
    inputValue: "",
    inputKey: 0,
    menuKind: null,
    menuTitle: "",
    menuItems: [],
    menuCursor: 0,
    menuHint: "",
    mcpEditorId: "",
    mcpEditorName: "",
    mcpEditorConfig: "",
    mcpEditorEnabled: true,
    mcpEditorFocusName: true,
    mcpTemplateCursor: 0,
    modelEditorName: "",
    modelEditorProvider: "openai" as ModelProvider,
    modelEditorBaseUrl: "",
    modelEditorApiKey: "",
    modelEditorModel: "",
    modelEditorFocusIndex: 0,
    modelEditorProviderSelect: false,
    approvalToolCallId: "",
    approvalToolName: "",
    approvalCommand: "",
    approvalParams: {},
    approvalReplyCh: null,
    pendingPlanId: "",
    pendingPlanContent: "",
    planConfirmCursor: 0,
    planModifyInput: "",
    planModifyInputKey: 0,
    apiURL,
    cliToken,
    version,
    cwd,
  };
}

export function reducer(state: AppState, action: AppAction): AppState {
  switch (action.type) {
    case "SET_VIEW":
      return { ...state, view: action.view as ViewMode };

    case "SET_INPUT":
      return {
        ...state,
        inputValue: action.value,
      };

    case "SET_INPUT_WITH_KEY":
      return {
        ...state,
        inputValue: action.value,
        inputKey: state.inputKey + 1,
      };

    case "SET_SESSION":
      return {
        ...state,
        sessionId: action.sessionId,
        sessionName: action.sessionName !== undefined
          ? action.sessionName
          : (action.sessionId !== state.sessionId ? "" : state.sessionName),
      };

    case "SET_SESSION_NAME":
      return { ...state, sessionName: action.sessionName };

    case "SET_MODEL":
      return { ...state, modelId: action.modelId, modelName: action.modelName };

    case "STREAM_START":
      return {
        ...state,
        streaming: true,
        assistantWaiting: true,
        liveAssistant: "",
        planReceived: false,
      };

    case "STREAM_CHUNK": {
      return {
        ...state,
        streaming: true,
        assistantWaiting: false,
        liveAssistant: state.liveAssistant + action.chunk,
      };
    }

    case "STREAM_DONE": {
      const entries = [...state.timeline];
      // In plan mode, PLAN_BODY already flushed liveAssistant — skip to avoid duplicate
      const hasPlanEntry = entries.some((e) => e.kind === "plan");
      if (!hasPlanEntry && state.liveAssistant.trim()) {
        entries.push({
          kind: "assistant",
          content: state.liveAssistant,
        });
      }
      if (action.error) {
        entries.push({
          kind: "system",
          content: `Error: ${action.error}`,
        });
      }
      return {
        ...state,
        streaming: false,
        assistantWaiting: false,
        liveAssistant: "",
        timeline: entries,
        planGenerating: false,
        planReceived: false,
      };
    }

    case "TOGGLE_COMPACT":
      return { ...state, compact: !state.compact };

    case "TOGGLE_TOOL_OUTPUT":
      return { ...state, toolOutputExpanded: !state.toolOutputExpanded };

    case "UPSERT_TOOL_ENTRY": {
      const nextEntry: TimelineEntry = { ...action.entry, kind: "tool" };
      if (!nextEntry.toolCallId) {
        return { ...state, timeline: [...state.timeline, nextEntry] };
      }
      const entries = [...state.timeline];
      const idx = entries.findIndex(
        (entry) => entry.kind === "tool" && entry.toolCallId === nextEntry.toolCallId,
      );
      if (idx === -1) {
        entries.push(nextEntry);
      } else {
        const prev = entries[idx];
        entries[idx] = {
          ...prev,
          ...nextEntry,
          subagentStream: nextEntry.subagentStream ?? prev.subagentStream,
        };
      }
      return { ...state, timeline: entries };
    }

    case "APPEND_SUBAGENT_STREAM": {
      const entries = [...state.timeline];
      const idx = entries.findIndex(
        (e) => e.kind === "tool" && e.toolCallId === action.parentToolCallId,
      );
      if (idx === -1) {
        return state;
      }
      const prev = entries[idx];
      entries[idx] = {
        ...prev,
        subagentStream: (prev.subagentStream || "") + action.content,
      };
      return { ...state, timeline: entries };
    }

    case "APPEND_ENTRY":
      return {
        ...state,
        timeline: [...state.timeline, { ...action.entry } as TimelineEntry],
      };

    case "RESET_SESSION":
      return {
        ...state,
        sessionId: "",
        sessionName: "",
        timeline: [],
        liveAssistant: "",
        streaming: false,
        assistantWaiting: false,
        toolOutputExpanded: false,
        planMode: false,
        planGenerating: false,
        planReceived: false,
        thinkingDetailContent: "",
        view: "chat",
        menuKind: null,
        menuTitle: "",
        menuItems: [],
        menuCursor: 0,
      };

    case "BLINK_TOGGLE":
      return { ...state, blinkOn: !state.blinkOn };

    case "CLEAR_TIMELINE":
      return { ...state, timeline: [] };

    case "SET_MENU":
      return {
        ...state,
        view: "menu",
        menuKind: action.kind as MenuKind,
        menuTitle: action.title,
        menuItems: action.items,
        menuCursor: 0,
        menuHint: action.hint,
      };

    case "MENU_NAV": {
      const delta = action.delta;
      const newCursor = Math.max(
        0,
        Math.min(state.menuItems.length - 1, state.menuCursor + delta),
      );
      return { ...state, menuCursor: newCursor };
    }

    case "CLOSE_MENU":
      return {
        ...state,
        view: "chat",
        menuKind: null,
        menuTitle: "",
        menuItems: [],
        menuCursor: 0,
      };

    case "SET_MCP_EDITOR":
      return {
        ...state,
        view: "mcp-editor",
        mcpEditorId: action.id,
        mcpEditorName: action.name,
        mcpEditorConfig: action.config,
        mcpEditorEnabled: action.enabled,
        mcpEditorFocusName: true,
      };

    case "SET_MCP_EDITOR_NAME":
      return { ...state, mcpEditorName: action.name };

    case "SET_MCP_EDITOR_CONFIG":
      return { ...state, mcpEditorConfig: action.config };

    case "TOGGLE_MCP_EDITOR_ENABLED":
      return { ...state, mcpEditorEnabled: !state.mcpEditorEnabled };

    case "TOGGLE_MCP_EDITOR_FOCUS":
      return { ...state, mcpEditorFocusName: !state.mcpEditorFocusName };

    case "SET_MCP_TEMPLATE_VIEW":
      return {
        ...state,
        view: "mcp-template",
        mcpTemplateCursor: 0,
      };

    case "MCP_TEMPLATE_NAV": {
      const count = 3;
      const newCursor = Math.max(0, Math.min(count - 1, state.mcpTemplateCursor + action.delta));
      return { ...state, mcpTemplateCursor: newCursor };
    }

    case "SET_MODEL_EDITOR_VIEW":
      return {
        ...state,
        view: "model-editor",
        modelEditorName: "",
        modelEditorProvider: "openai" as ModelProvider,
        modelEditorBaseUrl: "",
        modelEditorApiKey: "",
        modelEditorModel: "",
        modelEditorFocusIndex: 0,
        modelEditorProviderSelect: false,
      };

    case "SET_MODEL_EDITOR_NAME":
      return { ...state, modelEditorName: action.name };

    case "SET_MODEL_EDITOR_PROVIDER":
      return { ...state, modelEditorProvider: action.provider, modelEditorProviderSelect: false };

    case "SET_MODEL_EDITOR_BASE_URL":
      return { ...state, modelEditorBaseUrl: action.baseUrl };

    case "SET_MODEL_EDITOR_API_KEY":
      return { ...state, modelEditorApiKey: action.apiKey };

    case "SET_MODEL_EDITOR_MODEL":
      return { ...state, modelEditorModel: action.model };

    case "MODEL_EDITOR_NEXT_FIELD": {
      const maxIndex = 4;
      const next = Math.min(maxIndex, state.modelEditorFocusIndex + 1);
      return { ...state, modelEditorFocusIndex: next, modelEditorProviderSelect: false };
    }

    case "MODEL_EDITOR_PREV_FIELD": {
      const prev = Math.max(0, state.modelEditorFocusIndex - 1);
      return { ...state, modelEditorFocusIndex: prev, modelEditorProviderSelect: false };
    }

    case "TOGGLE_MODEL_EDITOR_PROVIDER_SELECT":
      return { ...state, modelEditorProviderSelect: !state.modelEditorProviderSelect };

    case "SET_APPROVAL":
      return {
        ...state,
        view: "approval",
        approvalToolCallId: action.toolCallId,
        approvalToolName: action.toolName,
        approvalCommand: action.command,
        approvalParams: action.params,
        approvalReplyCh: action.replyCh,
      };

    case "CLEAR_APPROVAL":
      return {
        ...state,
        view: "chat",
        approvalToolCallId: "",
        approvalToolName: "",
        approvalCommand: "",
        approvalParams: {},
        approvalReplyCh: null,
      };

    case "SET_APPROVAL_MODE":
      return { ...state, approvalMode: action.mode };

    case "SET_THINKING_LEVEL":
      return { ...state, thinkingLevel: action.level };

    case "LOAD_HISTORY":
      return {
        ...state,
        timeline: [...action.entries],
        streaming: false,
        assistantWaiting: false,
        liveAssistant: "",
      };

    case "THINKING_START":
      return {
        ...state,
        timeline: [
          ...state.timeline,
          { kind: "thinking", content: "", thinkingDone: false, thinkingStartedAt: Date.now() },
        ],
      };

    case "THINKING_CHUNK": {
      const entries = [...state.timeline];
      for (let i = entries.length - 1; i >= 0; i--) {
        if (entries[i].kind === "thinking" && !entries[i].thinkingDone) {
          entries[i] = { ...entries[i], content: entries[i].content + action.chunk };
          break;
        }
      }
      return { ...state, timeline: entries };
    }

    case "THINKING_DONE": {
      const entries = [...state.timeline];
      for (let i = entries.length - 1; i >= 0; i--) {
        if (entries[i].kind === "thinking" && !entries[i].thinkingDone) {
          const startedAt = entries[i].thinkingStartedAt;
          const durationMs = startedAt !== undefined
            ? Math.max(0, (action.finishedAt ?? Date.now()) - startedAt)
            : entries[i].thinkingDurationMs;
          entries[i] = { ...entries[i], thinkingDone: true, thinkingDurationMs: durationMs };
          break;
        }
      }
      return { ...state, timeline: entries };
    }

    case "VIEW_THINKING_DETAIL":
      return {
        ...state,
        view: "thinking-detail",
        thinkingDetailContent: action.content,
      };

    case "TOGGLE_PLAN_MODE":
      return { ...state, planMode: !state.planMode };

    case "SET_PLAN_CONFIRMATION":
      return {
        ...state,
        view: "plan-confirm",
        pendingPlanId: action.planId,
        pendingPlanContent: action.content,
        planConfirmCursor: 0,
        planModifyInput: "",
      };

    case "PLAN_CONFIRM_NAV": {
      const opts = 2;
      const newCursor = Math.max(0, Math.min(opts - 1, state.planConfirmCursor + action.delta));
      return { ...state, planConfirmCursor: newCursor };
    }

    case "SET_PLAN_MODIFY_INPUT":
      return { ...state, planModifyInput: action.value };

    case "CLEAR_PLAN_CONFIRMATION":
      return {
        ...state,
        view: "chat",
        pendingPlanId: "",
        pendingPlanContent: "",
        planConfirmCursor: 0,
        planModifyInput: "",
      };

    case "PLAN_START": {
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) {
        entries.push({ kind: "assistant", content: state.liveAssistant });
      }
      return { ...state, timeline: entries, liveAssistant: "", planGenerating: true };
    }

    case "PLAN_BODY": {
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) {
        entries.push({ kind: "assistant", content: state.liveAssistant });
      }
      entries.push({ kind: "plan", content: action.planBody });
      return { ...state, timeline: entries, liveAssistant: "", planGenerating: false, planReceived: true };
    }

    case "FLUSH_AND_WAIT": {
      const entries = [...state.timeline];
      if (state.liveAssistant.trim()) {
        entries.push({
          kind: "assistant",
          content: state.liveAssistant,
        });
      }
      return {
        ...state,
        assistantWaiting: true,
        liveAssistant: "",
        timeline: entries,
      };
    }

    default:
      return state;
  }
}
