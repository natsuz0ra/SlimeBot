import type { APIClient } from "../api/client.js";
import type { AppAction, AppState } from "../types.js";
import { formatContextTokenCount } from "../utils/contextSize.js";
import { formatTimestamp } from "../utils/format.js";

export type WorkspaceConsoleApi = Pick<APIClient, "listCheckpoints" | "updateSettings">;

interface WorkspaceConsoleControllerOptions {
  api: WorkspaceConsoleApi;
  dispatch: (action: AppAction) => void;
  appendSystem: (content: string) => void;
  getState: () => AppState;
  getSessionId: () => string;
  getOutputBudgetTokens: () => number;
  setOutputBudgetTokens: (tokens: number) => void;
  refreshContextUsage: (sessionId: string, modelId: string) => Promise<void>;
}

/** Checkpoint and usage command orchestration outside the root Ink component. */
export function createWorkspaceConsoleController(options: WorkspaceConsoleControllerOptions) {
  const {
    api, dispatch, appendSystem, getState, getSessionId,
    getOutputBudgetTokens, setOutputBudgetTokens, refreshContextUsage,
  } = options;

  const handleCheckpointCommand = async (input: string) => {
    const sessionId = getSessionId();
    if (!sessionId) {
      appendSystem("Create or open a session first.");
      return;
    }
    if (input === "list") {
      try {
        const list = await api.listCheckpoints(sessionId);
        if (list.length === 0) {
          appendSystem("No file checkpoints recorded for this session.");
          return;
        }
        appendSystem(list.map(checkpoint =>
          `${checkpoint.id.slice(0, 8)} · ${checkpoint.kind} · ${checkpoint.fileCount} file(s) · ${formatTimestamp(checkpoint.createdAt)}`,
        ).join("\n"));
      } catch (error) {
        appendSystem(`Failed to list checkpoints: ${(error as Error).message}`);
      }
      return;
    }
    dispatch({ type: "SET_VIEW", view: "checkpoint" });
  };

  const handleUsageCommand = async (input: string) => {
    const trimmed = input.trim();
    if (trimmed === "budget" || trimmed.startsWith("budget ")) {
      const raw = trimmed.slice(6).trim();
      if (!raw) {
        appendSystem(
          getOutputBudgetTokens() > 0
            ? `Current session output token budget: ${formatContextTokenCount(getOutputBudgetTokens())}. Use /usage budget <tokens|0> to change.`
            : "Session output token budget is unlimited. Use /usage budget <tokens> to set a limit.",
        );
        return;
      }
      const parsed = Number(raw.replace(/[,_]/g, ""));
      if (!Number.isFinite(parsed) || parsed < 0) {
        appendSystem("Usage: /usage budget <non-negative integer|0>");
        return;
      }
      const nextBudget = Math.round(parsed);
      try {
        await api.updateSettings({ sessionOutputTokenBudget: nextBudget });
        setOutputBudgetTokens(nextBudget);
        if (getSessionId() && getState().modelId) await refreshContextUsage(getSessionId(), getState().modelId);
        appendSystem(
          nextBudget > 0
            ? `Session output token budget set to ${formatContextTokenCount(nextBudget)}.`
            : "Session output token budget cleared (unlimited).",
        );
      } catch (error) {
        appendSystem(`Failed to update output budget: ${(error as Error).message}`);
      }
      return;
    }
    dispatch({ type: "SET_VIEW", view: "usage" });
  };

  return { handleCheckpointCommand, handleUsageCommand };
}
