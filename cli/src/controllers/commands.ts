import type React from "react";
import type { Key } from "ink";
import type { AppAction, AppState } from "../types.js";

/** Handles terminals that emit raw Ctrl+A-Z bytes without setting Ink's ctrl flag. */
function isCtrlKey(input: string, key: Key, letter: string): boolean {
  if (key.ctrl && input === letter) return true;
  const expected = letter.charCodeAt(0) - 96;
  return input.charCodeAt(0) === expected;
}

export function handleChatShortcut(input: string, key: Key, dispatch: React.Dispatch<AppAction>): boolean {
  if (isCtrlKey(input, key, "k")) {
    dispatch({ type: "TOGGLE_COMPACT" });
    return true;
  }
  if (isCtrlKey(input, key, "o")) {
    dispatch({ type: "TOGGLE_TOOL_OUTPUT" });
    return true;
  }
  return false;
}

export function getChatFooterHint(planMode: boolean, approvalMode: AppState["approvalMode"]): string {
  return planMode || approvalMode === "auto"
    ? "/ for commands | Shift+Tab to toggle | Esc to cancel"
    : "/ for commands | Shift+Tab plan mode | Esc to cancel";
}

export interface CliCommandHandlers {
  newSession: () => void;
  loadSessions: () => Promise<void>;
  loadModels: () => Promise<void>;
  loadSubagentModels: () => Promise<void>;
  toggleApprovalMode: () => Promise<void>;
  toggleThinkingLevel: () => void;
  setThinkingLevel: (level: string) => void;
  loadSkills: () => Promise<void>;
  loadMCPConfigs: () => Promise<void>;
  showHelp: () => void;
  togglePlanMode: () => void;
  unknownCommand: (cmd: string) => void;
}

export async function runCliCommand(raw: string, handlers: CliCommandHandlers): Promise<void> {
  const cmd = raw.trim();
  if (cmd === "/new") {
    handlers.newSession();
    return;
  }
  if (cmd === "/session") {
    await handlers.loadSessions();
    return;
  }
  if (cmd === "/model") {
    await handlers.loadModels();
    return;
  }
  if (cmd === "/subagent_model") {
    await handlers.loadSubagentModels();
    return;
  }
  if (cmd === "/approval") {
    await handlers.toggleApprovalMode();
    return;
  }
  if (cmd === "/effort") {
    handlers.toggleThinkingLevel();
    return;
  }
  if (cmd.startsWith("/effort ")) {
    handlers.setThinkingLevel(cmd.slice(8).trim());
    return;
  }
  if (cmd === "/skills") {
    await handlers.loadSkills();
    return;
  }
  if (cmd === "/mcp") {
    await handlers.loadMCPConfigs();
    return;
  }
  if (cmd === "/help") {
    handlers.showHelp();
    return;
  }
  if (cmd === "/plan") {
    handlers.togglePlanMode();
    return;
  }
  handlers.unknownCommand(cmd);
}
