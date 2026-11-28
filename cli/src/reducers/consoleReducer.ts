import type { AppAction, AppState } from "../types.js";
import { memoryConsoleActionCount } from "../utils/memoryConsole.js";

/** Independent memory and update consoles. */
export function consoleReducer(state: AppState, action: AppAction): AppState | null {
  switch (action.type) {
    case "SET_UPDATE_STATE":
      return {
        ...state,
        updateCheck: action.check !== undefined ? action.check : state.updateCheck,
        updateJob: action.job !== undefined ? action.job : state.updateJob,
        updateLoading: action.loading !== undefined ? action.loading : state.updateLoading,
        updateApplying: action.applying !== undefined ? action.applying : state.updateApplying,
        updateConfirming: action.confirming !== undefined ? action.confirming : state.updateConfirming,
      };
    case "SET_MEMORY_CONSOLE":
      return {
        ...state,
        view: "memory-console",
        memorySnapshot: action.snapshot !== undefined ? action.snapshot : state.memorySnapshot,
        memoryLoading: action.loading !== undefined ? action.loading : state.memoryLoading,
        memoryMessage: action.message !== undefined ? action.message : state.memoryMessage,
        memoryMode: action.loading ? "actions" : state.memoryMode,
        memoryCursor: action.loading ? 0 : state.memoryCursor,
      };
    case "MEMORY_CONSOLE_NAV": {
      const maxIndex = Math.max(0, memoryConsoleActionCount(state.memoryMode) - 1);
      return { ...state, memoryCursor: Math.max(0, Math.min(maxIndex, state.memoryCursor + action.delta)) };
    }
    case "MEMORY_CONSOLE_SET_MODE":
      return {
        ...state,
        memoryMode: action.mode,
        memoryCursor: action.cursor ?? 0,
        memoryEditingField: action.mode === "edit" ? state.memoryEditingField : null,
        memoryDraft: action.mode === "edit" ? state.memoryDraft : "",
        memoryViewTarget: action.mode === "view" ? state.memoryViewTarget : null,
        memoryMessage: action.message !== undefined ? action.message : state.memoryMessage,
      };
    case "MEMORY_CONSOLE_START_EDIT":
      return {
        ...state,
        memoryMode: "edit",
        memoryEditingField: action.field,
        memoryDraft: action.draft,
        memoryCursor: 0,
        memoryMessage: "",
      };
    case "MEMORY_CONSOLE_SET_DRAFT":
      return { ...state, memoryDraft: action.draft };
    case "MEMORY_CONSOLE_VIEW_TARGET":
      return {
        ...state,
        memoryMode: "view",
        memoryViewTarget: action.target,
        memoryCursor: 0,
        memoryMessage: "",
      };
    case "MEMORY_CONSOLE_MESSAGE":
      return { ...state, memoryMessage: action.message };
    default:
      return null;
  }
}
