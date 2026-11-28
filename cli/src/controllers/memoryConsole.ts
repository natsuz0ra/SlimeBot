import type { APIClient } from "../api/client.js";
import type { AppAction, AppState } from "../types.js";
import { formatMemorySnapshot, normalizeMemoryResetTarget } from "../utils/memory.js";
import {
  MEMORY_CONSOLE_ACTIONS,
  MEMORY_CONSOLE_EDIT_LIMITS,
  MEMORY_CONSOLE_RESET_ACTIONS,
  parseMemoryConsoleDraft,
  type MemoryConsoleEditField,
} from "../utils/memoryConsole.js";

export type MemoryConsoleApi = Pick<APIClient, "getMemory" | "clearMemory" | "updateSettings">;

interface MemoryConsoleControllerOptions {
  api: MemoryConsoleApi;
  dispatch: (action: AppAction) => void;
  appendSystem: (content: string) => void;
  getState: () => AppState;
}

/** API orchestration for the dedicated memory console; rendering stays in MemoryConsoleView. */
export function createMemoryConsoleController({ api, dispatch, appendSystem, getState }: MemoryConsoleControllerOptions) {
  const refreshMemoryConsole = async (message = "") => {
    try {
      const snapshot = await api.getMemory();
      dispatch({ type: "SET_MEMORY_CONSOLE", snapshot, loading: false, message });
    } catch (error) {
      dispatch({ type: "SET_MEMORY_CONSOLE", loading: false, message: `Failed to refresh memory: ${(error as Error).message}` });
    }
  };

  const loadMemory = async () => {
    dispatch({ type: "SET_MEMORY_CONSOLE", loading: true, message: "" });
    try {
      const snapshot = await api.getMemory();
      dispatch({ type: "SET_MEMORY_CONSOLE", snapshot, loading: false, message: "" });
    } catch (error) {
      dispatch({ type: "SET_MEMORY_CONSOLE", loading: false, message: `Failed to load memory: ${(error as Error).message}` });
    }
  };

  const resetMemory = async (targetText: string) => {
    const target = normalizeMemoryResetTarget(targetText);
    if (!target) {
      appendSystem("Usage: /memory reset memory|user|all");
      return;
    }
    try {
      await api.clearMemory(target);
      appendSystem(`Memory reset: ${target}`);
      appendSystem(formatMemorySnapshot(await api.getMemory()));
    } catch (error) {
      appendSystem(`Failed to reset memory: ${(error as Error).message}`);
    }
  };

  const startMemoryConsoleEdit = (field: MemoryConsoleEditField) => {
    const snapshot = getState().memorySnapshot;
    if (!snapshot) return;
    const draft = field === "memoryCharLimit"
      ? String(snapshot.memory.charLimit)
      : field === "memoryUserCharLimit"
        ? String(snapshot.user.charLimit)
        : String(snapshot.memoryNudgeInterval);
    dispatch({ type: "MEMORY_CONSOLE_START_EDIT", field, draft });
  };

  const handleMemoryConsoleSelect = async () => {
    const state = getState();
    const snapshot = state.memorySnapshot;
    if (!snapshot || state.memoryLoading) return;

    if (state.memoryMode === "reset") {
      const item = MEMORY_CONSOLE_RESET_ACTIONS[state.memoryCursor];
      if (!item || item.action === "cancel") {
        dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions", message: "" });
        return;
      }
      try {
        await api.clearMemory(item.action);
        await refreshMemoryConsole(`Memory reset: ${item.action}.`);
        dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions" });
      } catch (error) {
        dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: `Failed to reset memory: ${(error as Error).message}` });
      }
      return;
    }

    if (state.memoryMode === "view") {
      dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions", message: "" });
      return;
    }
    if (state.memoryMode !== "actions") return;

    const item = MEMORY_CONSOLE_ACTIONS[state.memoryCursor];
    if (!item) return;
    try {
      if (item.action === "toggle-memory") {
        await api.updateSettings({ memoryEnabled: !snapshot.memoryEnabled });
        await refreshMemoryConsole(`Long-term memory ${snapshot.memoryEnabled ? "disabled" : "enabled"}.`);
        return;
      }
      if (item.action === "toggle-user-profile") {
        await api.updateSettings({ memoryUserProfileEnabled: !snapshot.memoryUserProfileEnabled });
        await refreshMemoryConsole(`User profile memory ${snapshot.memoryUserProfileEnabled ? "disabled" : "enabled"}.`);
        return;
      }
      if (item.action === "edit-memory-budget") return startMemoryConsoleEdit("memoryCharLimit");
      if (item.action === "edit-user-budget") return startMemoryConsoleEdit("memoryUserCharLimit");
      if (item.action === "edit-review-interval") return startMemoryConsoleEdit("memoryNudgeInterval");
      if (item.action === "view-memory") {
        dispatch({ type: "MEMORY_CONSOLE_VIEW_TARGET", target: "memory" });
        return;
      }
      if (item.action === "view-user") {
        dispatch({ type: "MEMORY_CONSOLE_VIEW_TARGET", target: "user" });
        return;
      }
      if (item.action === "reset") dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "reset", message: "" });
    } catch (error) {
      dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: `Memory action failed: ${(error as Error).message}` });
    }
  };

  const saveMemoryConsoleDraft = async () => {
    const state = getState();
    const field = state.memoryEditingField;
    if (!field) return;
    const parsed = parseMemoryConsoleDraft(field, state.memoryDraft);
    if (parsed.value === null) {
      dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: parsed.error });
      return;
    }
    try {
      await api.updateSettings({ [field]: parsed.value });
      const label = MEMORY_CONSOLE_EDIT_LIMITS[field].label;
      await refreshMemoryConsole(`${label} set to ${parsed.value}.`);
      dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions" });
    } catch (error) {
      dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: `Failed to save memory setting: ${(error as Error).message}` });
    }
  };

  return {
    loadMemory,
    resetMemory,
    refreshMemoryConsole,
    startMemoryConsoleEdit,
    handleMemoryConsoleSelect,
    saveMemoryConsoleDraft,
  };
}
