/**
 * App — Ink CLI root component.
 * Centralizes state, WebSocket events, menu behavior, and keyboard input dispatch.
 */

import React, { useCallback, useEffect, useReducer, useRef } from "react";
import { Box, Text, forceRedraw, useApp, useStdout } from "ink";
import { APIClient } from "./api/client.js";
import { ApprovalView } from "./components/ApprovalView.js";
import { PlanConfirmView } from "./components/PlanConfirmView.js";
import QuestionAnswerView from "./components/QuestionAnswerView.js";
import { Banner } from "./components/Banner.js";
import { CommandHints } from "./components/CommandHints.js";
import { MCPEditor } from "./components/MCPEditor.js";
import { MCPTemplatePicker } from "./components/MCPTemplatePicker.js";
import { MCPToolsView } from "./components/MCPToolsView.js";
import { CLI_HINT_COLOR, MenuView, MENU_VISIBLE_LIMIT } from "./components/MenuView.js";
import MemoryConsoleView from "./components/MemoryConsoleView.js";
import TeamView from "./components/TeamView.js";
import { ModelEditor } from "./components/ModelEditor.js";
import { ProviderEditor } from "./components/ProviderEditor.js";
import { TextInput } from "./components/TextInput.js";
import { Timeline } from "./components/Timeline.js";
import { UpdateView, getInitialUpdateJobForView, isUpdateActive } from "./components/UpdateView.js";
import { getChatFooterHint, handleChatShortcut, runCliCommand } from "./controllers/commands.js";
import { useCliKeyboard } from "./hooks/useCliKeyboard.js";
import { clampContextSize, formatContextSize, formatContextUsageStatus } from "./utils/contextSize.js";
import { useCliSocket } from "./hooks/useCliSocket.js";
import { reducer, createInitialState } from "./reducer.js";
import { completeCommand, isCommand, matchCommandHints, moveCommandHintCursor } from "./utils/commands.js";
import { formatTimestamp, formatWaitingStatsSuffix } from "./utils/format.js";
import { getAgentTeamRuns } from "./utils/agentTeam.js";
import { mapHistoryMessages } from "./utils/history.js";
import { buildSandboxMenuItems, type SandboxMenuAction } from "./utils/sandboxSettings.js";
import { formatMemorySnapshot, normalizeMemoryResetTarget } from "./utils/memory.js";
import {
  MEMORY_CONSOLE_ACTIONS,
  MEMORY_CONSOLE_EDIT_LIMITS,
  MEMORY_CONSOLE_RESET_ACTIONS,
  parseMemoryConsoleDraft,
  type MemoryConsoleEditField,
} from "./utils/memoryConsole.js";
import { CLI_ACCENT_COLOR, clearScreen, setTerminalTitle } from "./utils/terminal.js";
import { SHOW_CLI_THINKING } from "./utils/timelineFormat.js";
import { CLISocket } from "./ws/socket.js";
import type {
  AppAction,
  LLMConfig,
  LLMProvider,
  DiscoveredModel,
  MCPConfig,
  MCPTemplate,
  MenuItem,
  ModelProvider,
  Session,
  Skill,
} from "./types.js";
import { THINKING_LEVELS } from "./types.js";

export { getChatFooterHint, handleChatShortcut } from "./controllers/commands.js";

const MODEL_PROVIDER_ORDER: ModelProvider[] = ["openai", "anthropic", "deepseek"];

function moveModelProvider(current: ModelProvider, delta: number): ModelProvider {
  const currentIndex = MODEL_PROVIDER_ORDER.indexOf(current);
  const start = currentIndex >= 0 ? currentIndex : 0;
  const next = (start + delta + MODEL_PROVIDER_ORDER.length) % MODEL_PROVIDER_ORDER.length;
  return MODEL_PROVIDER_ORDER[next];
}

function formatModelLine(model?: Pick<LLMConfig, "name" | "model">, fallback = "(none)"): string {
  if (!model) return fallback;
  const name = model.name?.trim() || "";
  const actualModel = model.model?.trim() || "";
  if (name && actualModel) return `${name} · ${actualModel}`;
  return name || actualModel || fallback;
}

interface AppProps {
  apiURL: string;
  cliToken: string;
  version: string;
}

export function App({ apiURL, cliToken, version }: AppProps): React.ReactElement {
  const { exit } = useApp();
  const { stdout } = useStdout();
  const [width, setWidth] = React.useState(() => Math.max(20, stdout?.columns || 80));
  const [commandHintCursor, setCommandHintCursor] = React.useState(0);
  const border = "─".repeat(width);

  const [state, dispatch] = useReducer(
    reducer,
    createInitialState(apiURL, cliToken, process.cwd(), version),
  );

  const apiRef = useRef(new APIClient(apiURL, cliToken));
  const providerMenuRef = useRef<LLMProvider | null>(null);
  const pendingProviderDeleteRef = useRef<{ id: string; at: number } | null>(null);
  const socketRef = useRef<CLISocket | null>(null);
  const blinkRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sessionRef = useRef({ id: "", name: "" });
  const liveAssistantRef = useRef("");
  const planModeRef = useRef(false);
  const planStartedRef = useRef(false);
  const preambleShownRef = useRef("");
  const commandHints = React.useMemo(() => matchCommandHints(state.inputValue), [state.inputValue]);
  const teamRuns = React.useMemo(() => getAgentTeamRuns(state.timeline), [state.timeline]);
  const hasCommandHints = state.view === "chat" && !state.streaming && commandHints.length > 0;
  const selectedCommandHintIndex = commandHints.length > 0
    ? Math.max(0, Math.min(commandHints.length - 1, commandHintCursor))
    : 0;

  React.useEffect(() => {
    setCommandHintCursor(0);
  }, [state.inputValue]);

  const redrawTerminal = useCallback(() => {
    setImmediate(() => {
      if (!forceRedraw(stdout, { clearScrollback: true })) {
        clearScreen();
      }
    });
  }, [stdout]);

  const appendSystem = useCallback((content: string) => {
    dispatch({ type: "APPEND_ENTRY", entry: { kind: "system", content } } as AppAction);
  }, []);

  const refreshContextUsage = useCallback(async (sessionId: string, modelId: string) => {
    if (!sessionId || !modelId) return;
    try {
      const usage = await apiRef.current.getContextUsage(sessionId, modelId);
      if (sessionRef.current.id === sessionId) {
        dispatch({ type: "CONTEXT_USAGE", usage } as AppAction);
      }
    } catch {
      // Context usage is informational; keep chat usable if it cannot be loaded.
    }
  }, []);

  const applyTerminalTitle = useCallback((sessionName = "") => {
    setTerminalTitle(sessionName.trim() || "SlimeBot CLI");
  }, []);

  useEffect(() => {
    sessionRef.current = { id: state.sessionId, name: state.sessionName };
  }, [state.sessionId, state.sessionName]);

  useEffect(() => {
    liveAssistantRef.current = state.liveAssistant;
  }, [state.liveAssistant]);

  useEffect(() => {
    planModeRef.current = state.planMode;
  }, [state.planMode]);

  useEffect(() => {
    setCommandHintCursor(0);
  }, [state.inputValue]);

  const refreshSessionName = useCallback(async (sessionId: string) => {
    if (!sessionId) {
      dispatch({ type: "SET_SESSION_NAME", sessionName: "" } as AppAction);
      applyTerminalTitle("");
      return;
    }
    try {
      const resp = await apiRef.current.listSessions(200, 0);
      const session = resp.sessions.find((s) => s.id === sessionId);
      if (!session) return;
      dispatch({ type: "SET_SESSION_NAME", sessionName: session.name } as AppAction);
      applyTerminalTitle(session.name);
    } catch {
      // Ignore session name refresh failures.
    }
  }, [applyTerminalTitle]);

  const loadDefaultModel = useCallback(async () => {
    try {
      const settings = await apiRef.current.getSettings();
      const configs = await apiRef.current.listLLMConfigs();
      const model = configs.find((c) => c.id === settings.defaultModel) ?? configs[0];
      const modelId = model?.id ?? "";
      if (settings.defaultModel && settings.defaultModel !== modelId) {
        await apiRef.current.updateSettings({ defaultModel: modelId });
      }
      dispatch({
        type: "SET_MODEL",
        modelId,
        modelName: model ? formatModelLine(model, modelId) : "",
      } as AppAction);
    } catch {
      // Ignore when no model is configured.
    }
  }, []);

  const loadApprovalMode = useCallback(async () => {
    try {
      const settings = await apiRef.current.getSettings();
      const mode = settings.approvalMode || "standard";
      dispatch({ type: "SET_APPROVAL_MODE", mode } as AppAction);
    } catch {
      // Ignore
    }
  }, []);

  const loadThinkingLevel = useCallback(async () => {
    try {
      const settings = await apiRef.current.getSettings();
      const level = (settings as Record<string, unknown>).thinkingLevel as string || "off";
      if ((THINKING_LEVELS as readonly string[]).includes(level)) {
        dispatch({ type: "SET_THINKING_LEVEL", level } as AppAction);
      }
    } catch {
      // Ignore
    }
  }, []);

  const toggleApprovalMode = useCallback(async () => {
    try {
      const settings = await apiRef.current.getSettings();
      const current = settings.approvalMode || "standard";
      const next = current === "standard" ? "auto_review" : current === "auto_review" ? "auto" : "standard";
      await apiRef.current.updateSettings({ approvalMode: next });
      dispatch({ type: "SET_APPROVAL_MODE", mode: next } as AppAction);
      const label = next === "auto" ? "Auto Execute" : next === "auto_review" ? "Auto Review" : "Standard";
      appendSystem(`Approval mode switched to: ${label}`);
    } catch (error) {
      appendSystem(`Failed to switch approval mode: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const THINKING_LEVEL_DESC: Record<string, string> = {
    off: "No extended thinking",
    low: "Light reasoning (8K budget)",
    medium: "Moderate reasoning (16K budget)",
    high: "Deep reasoning (32K budget)",
    max: "Maximum reasoning (64K budget or provider max)",
  };

  const toggleThinkingLevel = useCallback(() => {
    const items: MenuItem[] = THINKING_LEVELS.map((level) => ({
      title: level,
      desc: (level === state.thinkingLevel ? "current · " : "") + (THINKING_LEVEL_DESC[level] || ""),
      data: level,
    }));
    dispatch({
      type: "SET_MENU",
      kind: "effort",
      title: "Thinking Level",
      items,
      hint: "Arrow keys to navigate | Enter to select | Esc to cancel",
    } as AppAction);
  }, [state.thinkingLevel]);

  const setThinkingLevel = useCallback(async (level: string) => {
    const normalized = level.toLowerCase().trim();
    if (!(THINKING_LEVELS as readonly string[]).includes(normalized)) {
      appendSystem(`Invalid thinking level: ${level}. Use: ${THINKING_LEVELS.join(", ")}`);
      return;
    }
    dispatch({ type: "SET_THINKING_LEVEL", level: normalized } as AppAction);
    appendSystem(`Thinking level set to: ${normalized}`);
    try {
      await apiRef.current.updateSettings({ thinkingLevel: normalized });
    } catch {
      // Silently ignore persistence failures.
    }
  }, [appendSystem]);

  const loadSessions = useCallback(async () => {
    try {
      const resp = await apiRef.current.listSessions(200, 0);
      const items: MenuItem[] = resp.sessions
        .filter((s) => s.id !== "im-platform-session")
        .map((s) => ({
          title: s.name,
          desc: formatTimestamp(s.updatedAt),
          data: s,
        }));
      dispatch({
        type: "SET_MENU",
        kind: "session",
        title: "Session Menu",
        items,
        hint: "Arrow keys to navigate | Enter to switch | D to delete | Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load sessions: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadModels = useCallback(async () => {
    providerMenuRef.current = null;
    try {
      const configs = await apiRef.current.listLLMConfigs();
      const items: MenuItem[] = configs.map((c) => ({
        title: c.name,
        desc: `${c.providerName || c.provider} · ${c.model} · ${formatContextSize(c.contextSize || 1_000_000)}`,
        data: c,
      }));
      dispatch({
        type: "SET_MENU",
        kind: "model",
        title: "Model Menu",
        items,
        hint: "Enter switch | A providers | E edit | D delete | Esc close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load models: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadProviders = useCallback(async () => {
    try {
      const providers = await apiRef.current.listLLMProviders();
      dispatch({
        type: "SET_MENU", kind: "provider", title: "Model Providers",
        items: providers.map(provider => ({ title: provider.name, desc: `${provider.protocol} · ${provider.baseUrl}`, data: provider })),
        hint: "Enter models | A add | E edit | D delete | R discover | Esc back",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load providers: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadProviderModels = useCallback(async (provider: LLMProvider) => {
    providerMenuRef.current = provider;
    try {
      const models = (await apiRef.current.listLLMConfigs()).filter(model => model.providerId === provider.id);
      dispatch({
        type: "SET_MENU", kind: "provider-model", title: provider.name,
        items: models.map(model => ({ title: model.name, desc: `${model.model} · ${formatContextSize(model.contextSize || 1_000_000)}`, data: model })),
        hint: "Enter edit | A add manually | D delete | R discover | Esc providers",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load provider models: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadDiscoveredModels = useCallback(async (provider: LLMProvider) => {
    providerMenuRef.current = provider;
    try {
      const [found, current] = await Promise.all([apiRef.current.discoverLLMModels(provider.id), apiRef.current.listLLMConfigs()]);
      const existing = new Set(current.filter(model => model.providerId === provider.id).map(model => model.model));
      dispatch({
        type: "SET_MENU", kind: "discovered-model", title: `${provider.name} · Discovery`,
        items: found.map(model => ({ title: model.name, desc: existing.has(model.id) ? `${model.id} · already added` : model.id, data: { ...model, added: existing.has(model.id) } })),
        hint: "Enter add model | Esc provider models",
      } as AppAction);
    } catch (error) {
      appendSystem(`Model discovery failed: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadSubagentModels = useCallback(async () => {
    try {
      const configs = await apiRef.current.listLLMConfigs();
      const items: MenuItem[] = [
        { title: "Follow Main Agent", desc: "Inherit parent model", data: { id: "", name: "Follow Main Agent" } },
        ...configs.map((c) => ({
          title: c.name,
          desc: `${c.providerName || c.provider} · ${c.model}`,
          data: c,
        })),
      ];
      dispatch({
        type: "SET_MENU",
        kind: "subagent_model",
        title: "Sub-agent Model",
        items,
        hint: "Arrow keys to navigate | Enter to select | Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load subagent models: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadSkills = useCallback(async () => {
    try {
      const skills = await apiRef.current.listSkills();
      const items: MenuItem[] = skills.map((s: Skill) => ({
        title: s.name,
        desc: `${s.sourceLabel || "SlimeBot"}${s.readOnly ? " · read-only" : ""}${s.enabled === false ? " · disabled" : ""} · ${s.description}`,
        data: s,
      }));
      dispatch({
        type: "SET_MENU",
        kind: "skills",
        title: "Skills Menu",
        items,
        hint: "Arrow keys to navigate | D to delete | Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load skills: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadMCPConfigs = useCallback(async () => {
    try {
      const configs = await apiRef.current.listMCPConfigs();
      const items: MenuItem[] = configs.map((c) => ({
        title: c.name,
        desc: c.isEnabled ? "enabled" : "disabled",
        data: c,
      }));
      dispatch({
        type: "SET_MENU",
        kind: "mcp",
        title: "MCP Menu",
        items,
        hint: "Arrow keys to navigate | Enter or E to edit | T tools | A to add | Space to toggle | D to delete | Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load MCP configs: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadSandboxSettings = useCallback(async () => {
    try {
      const settings = await apiRef.current.getSettings();
      dispatch({
        type: "SET_MENU",
        kind: "sandbox",
        title: "Sandbox Settings",
        items: buildSandboxMenuItems(settings),
        hint: "Arrow keys to navigate | Enter to apply | Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load sandbox settings: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadMemory = useCallback(async () => {
    dispatch({ type: "SET_MEMORY_CONSOLE", loading: true, message: "" } as AppAction);
    try {
      const snapshot = await apiRef.current.getMemory();
      dispatch({ type: "SET_MEMORY_CONSOLE", snapshot, loading: false, message: "" } as AppAction);
    } catch (error) {
      dispatch({ type: "SET_MEMORY_CONSOLE", loading: false, message: `Failed to load memory: ${(error as Error).message}` } as AppAction);
    }
  }, []);

  const resetMemory = useCallback(async (targetText: string) => {
    const target = normalizeMemoryResetTarget(targetText);
    if (!target) {
      appendSystem("Usage: /memory reset memory|user|all");
      return;
    }
    try {
      await apiRef.current.clearMemory(target);
      appendSystem(`Memory reset: ${target}`);
      const snapshot = await apiRef.current.getMemory();
      appendSystem(formatMemorySnapshot(snapshot));
    } catch (error) {
      appendSystem(`Failed to reset memory: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const refreshMemoryConsole = useCallback(async (message = "") => {
    try {
      const snapshot = await apiRef.current.getMemory();
      dispatch({ type: "SET_MEMORY_CONSOLE", snapshot, loading: false, message } as AppAction);
    } catch (error) {
      dispatch({ type: "SET_MEMORY_CONSOLE", loading: false, message: `Failed to refresh memory: ${(error as Error).message}` } as AppAction);
    }
  }, []);

  const startMemoryConsoleEdit = useCallback((field: MemoryConsoleEditField) => {
    const snapshot = state.memorySnapshot;
    if (!snapshot) return;
    const draft = field === "memoryCharLimit"
      ? String(snapshot.memory.charLimit)
      : field === "memoryUserCharLimit"
        ? String(snapshot.user.charLimit)
        : String(snapshot.memoryNudgeInterval);
    dispatch({ type: "MEMORY_CONSOLE_START_EDIT", field, draft } as AppAction);
  }, [state.memorySnapshot]);

  const handleMemoryConsoleSelect = useCallback(async () => {
    const snapshot = state.memorySnapshot;
    if (!snapshot || state.memoryLoading) return;

    if (state.memoryMode === "reset") {
      const item = MEMORY_CONSOLE_RESET_ACTIONS[state.memoryCursor];
      if (!item || item.action === "cancel") {
        dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions", message: "" } as AppAction);
        return;
      }
      try {
        await apiRef.current.clearMemory(item.action);
        await refreshMemoryConsole(`Memory reset: ${item.action}.`);
        dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions" } as AppAction);
      } catch (error) {
        dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: `Failed to reset memory: ${(error as Error).message}` } as AppAction);
      }
      return;
    }

    if (state.memoryMode === "view") {
      dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions", message: "" } as AppAction);
      return;
    }

    if (state.memoryMode !== "actions") return;

    const item = MEMORY_CONSOLE_ACTIONS[state.memoryCursor];
    if (!item) return;
    try {
      if (item.action === "toggle-memory") {
        await apiRef.current.updateSettings({ memoryEnabled: !snapshot.memoryEnabled });
        await refreshMemoryConsole(`Long-term memory ${snapshot.memoryEnabled ? "disabled" : "enabled"}.`);
        return;
      }
      if (item.action === "toggle-user-profile") {
        await apiRef.current.updateSettings({ memoryUserProfileEnabled: !snapshot.memoryUserProfileEnabled });
        await refreshMemoryConsole(`User profile memory ${snapshot.memoryUserProfileEnabled ? "disabled" : "enabled"}.`);
        return;
      }
      if (item.action === "edit-memory-budget") {
        startMemoryConsoleEdit("memoryCharLimit");
        return;
      }
      if (item.action === "edit-user-budget") {
        startMemoryConsoleEdit("memoryUserCharLimit");
        return;
      }
      if (item.action === "edit-review-interval") {
        startMemoryConsoleEdit("memoryNudgeInterval");
        return;
      }
      if (item.action === "view-memory") {
        dispatch({ type: "MEMORY_CONSOLE_VIEW_TARGET", target: "memory" } as AppAction);
        return;
      }
      if (item.action === "view-user") {
        dispatch({ type: "MEMORY_CONSOLE_VIEW_TARGET", target: "user" } as AppAction);
        return;
      }
      if (item.action === "reset") {
        dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "reset", message: "" } as AppAction);
      }
    } catch (error) {
      dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: `Memory action failed: ${(error as Error).message}` } as AppAction);
    }
  }, [refreshMemoryConsole, startMemoryConsoleEdit, state.memoryCursor, state.memoryLoading, state.memoryMode, state.memorySnapshot]);

  const saveMemoryConsoleDraft = useCallback(async () => {
    const field = state.memoryEditingField;
    if (!field) return;
    const parsed = parseMemoryConsoleDraft(field, state.memoryDraft);
    if (parsed.value === null) {
      dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: parsed.error } as AppAction);
      return;
    }
    try {
      await apiRef.current.updateSettings({ [field]: parsed.value });
      const label = MEMORY_CONSOLE_EDIT_LIMITS[field].label;
      await refreshMemoryConsole(`${label} set to ${parsed.value}.`);
      dispatch({ type: "MEMORY_CONSOLE_SET_MODE", mode: "actions" } as AppAction);
    } catch (error) {
      dispatch({ type: "MEMORY_CONSOLE_MESSAGE", message: `Failed to save memory setting: ${(error as Error).message}` } as AppAction);
    }
  }, [refreshMemoryConsole, state.memoryDraft, state.memoryEditingField]);

  const loadUpdate = useCallback(async () => {
    dispatch({ type: "SET_VIEW", view: "update" } as AppAction);
    dispatch({ type: "SET_UPDATE_STATE", loading: true, confirming: false } as AppAction);
    try {
      const check = await apiRef.current.getUpdateCheck(true);
      const job = await apiRef.current.getUpdateJob();
      const visibleJob = getInitialUpdateJobForView(job);
      dispatch({ type: "SET_UPDATE_STATE", check, job: visibleJob, loading: false, applying: isUpdateActive(visibleJob) } as AppAction);
    } catch (error) {
      dispatch({ type: "SET_UPDATE_STATE", loading: false, applying: false } as AppAction);
      appendSystem(`Failed to load update status: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const checkForUpdatesSilently = useCallback(async () => {
    try {
      const check = await apiRef.current.getUpdateCheck(false);
      dispatch({ type: "SET_UPDATE_STATE", check } as AppAction);
    } catch {
      // Update checks are informational; keep startup quiet.
    }
  }, []);

  const applyUpdate = useCallback(async () => {
    if (!state.updateCheck?.canApply || !state.updateCheck.latest || state.updateApplying) {
      appendSystem("No applicable update is available.");
      return;
    }
    dispatch({ type: "SET_UPDATE_STATE", applying: true, confirming: false } as AppAction);
    try {
      const job = await apiRef.current.applyUpdate(state.updateCheck.latest);
      dispatch({ type: "SET_UPDATE_STATE", job, applying: true } as AppAction);
      appendSystem(`Update started for ${state.updateCheck.latest}. The update view will keep showing progress while the service is reachable.`);
    } catch (error) {
      dispatch({ type: "SET_UPDATE_STATE", applying: false } as AppAction);
      appendSystem(`Failed to start update: ${(error as Error).message}`);
    }
  }, [appendSystem, state.updateApplying, state.updateCheck]);

  useEffect(() => {
    if (!state.updateApplying) return;
    let cancelled = false;
    const poll = async () => {
      try {
        const job = await apiRef.current.getUpdateJob();
        if (cancelled) return;
        dispatch({ type: "SET_UPDATE_STATE", job, applying: isUpdateActive(job) } as AppAction);
      } catch {
        if (!cancelled) {
          dispatch({ type: "SET_UPDATE_STATE", applying: false } as AppAction);
        }
      }
    };
    const timer = setInterval(() => { void poll(); }, 1500);
    void poll();
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [state.updateApplying]);

  const showHelp = useCallback(() => {
    const items: MenuItem[] = [
      { title: "/new", desc: "Create a new chat (lazy session creation)", data: null },
      { title: "/session", desc: "Browse, switch, or delete sessions", data: null },
      { title: "/model", desc: "Switch default model", data: null },
      { title: "/provider", desc: "Manage model providers and available models", data: null },
      { title: "/memory", desc: "Open memory console", data: null },
      { title: "/approval", desc: "Toggle approval mode (standard/auto review/auto)", data: null },
      { title: "/effort", desc: "Toggle thinking level (off/low/medium/high)", data: null },
      { title: "/sandbox", desc: "Configure sandbox mode and network access", data: null },
      { title: "/update", desc: "Check and apply SlimeBot updates", data: null },
      { title: "/skills", desc: "Browse and delete installed skills", data: null },
      { title: "/mcp", desc: "Manage MCP configs", data: null },
      { title: "/help", desc: "Show available commands", data: null },
    ];
    dispatch({
      type: "SET_MENU",
      kind: "help",
      title: "Help",
      items,
      hint: "Esc to return to chat",
    } as AppAction);
  }, []);

  const switchSession = useCallback(async (session: Session) => {
    sessionRef.current = { id: session.id, name: session.name };
    dispatch({ type: "SET_SESSION", sessionId: session.id, sessionName: session.name } as AppAction);
    dispatch({ type: "CLOSE_MENU" } as AppAction);
    applyTerminalTitle(session.name);
    redrawTerminal();
    try {
      const history = await apiRef.current.getSessionMessages(session.id);
      dispatch({
        type: "LOAD_HISTORY",
        entries: mapHistoryMessages(
          history.messages,
          history.toolCallsByAssistantMessageId || {},
          history.thinkingByAssistantMessageId || {},
          history.teamRuns || [],
          history.teamMemberRuns || [],
        ),
      } as AppAction);
      await refreshContextUsage(session.id, state.modelId);
    } catch (error) {
      appendSystem(`Failed to load session history: ${(error as Error).message}`);
    }
  }, [appendSystem, applyTerminalTitle, redrawTerminal, refreshContextUsage, state.modelId]);

  const handleMenuSelect = useCallback(async (item: MenuItem | undefined) => {
    if (!item || !state.menuKind) return;

    try {
      if (state.menuKind === "session") {
        await switchSession(item.data as Session);
        return;
      }
      if (state.menuKind === "model") {
        const model = item.data as LLMConfig;
        await apiRef.current.updateSettings({ defaultModel: model.id });
        dispatch({
          type: "SET_MODEL",
          modelId: model.id,
          modelName: formatModelLine(model, model.id),
        } as AppAction);
        dispatch({ type: "CLOSE_MENU" } as AppAction);
        appendSystem(`Model switched to ${model.name}.`);
        if (state.sessionId) {
          await refreshContextUsage(state.sessionId, model.id);
        }
        return;
      }
      if (state.menuKind === "provider") {
        await loadProviderModels(item.data as LLMProvider);
        return;
      }
      if (state.menuKind === "provider-model") {
        dispatch({ type: "SET_MODEL_EDITOR", config: item.data as LLMConfig } as AppAction);
        return;
      }
      if (state.menuKind === "discovered-model") {
        const model = item.data as DiscoveredModel & { added: boolean };
        const provider = providerMenuRef.current;
        if (!provider || model.added) return;
        await apiRef.current.createLLMConfig({ providerId: provider.id, name: model.name, model: model.id, contextSize: clampContextSize(model.contextSize || 1_000_000) });
        appendSystem(`Added ${model.name} to ${provider.name}.`);
        await loadProviderModels(provider);
        return;
      }
      if (state.menuKind === "subagent_model") {
        const model = item.data as { id: string; name: string };
        dispatch({
          type: "SET_SUBAGENT_MODEL",
          modelId: model.id,
          modelName: model.id ? model.name : "Follow Main Agent",
        } as AppAction);
        dispatch({ type: "CLOSE_MENU" } as AppAction);
        appendSystem(model.id ? `Sub-agent model set to ${model.name}.` : `Sub-agent model set to follow main agent.`);
        return;
      }
      if (state.menuKind === "effort") {
        const level = item.data as string;
        dispatch({ type: "SET_THINKING_LEVEL", level } as AppAction);
        dispatch({ type: "CLOSE_MENU" } as AppAction);
        appendSystem(`Thinking level set to: ${level}`);
        try {
          await apiRef.current.updateSettings({ thinkingLevel: level });
        } catch {
          // Silently ignore persistence failures.
        }
        return;
      }
      if (state.menuKind === "sandbox") {
        const action = item.data as SandboxMenuAction;
        if (action.type === "mode") {
          await apiRef.current.updateSettings({ cliSandboxMode: action.mode });
          appendSystem(`Sandbox mode set to: ${action.mode}`);
        } else {
          await apiRef.current.updateSettings({ cliSandboxNetworkEnabled: action.enabled });
          appendSystem(`Sandbox network access ${action.enabled ? "enabled" : "disabled"}.`);
        }
        await loadSandboxSettings();
        return;
      }
      if (state.menuKind === "mcp") {
        const mcp = item.data as MCPConfig;
        dispatch({
          type: "SET_MCP_EDITOR",
          id: mcp.id,
          name: mcp.name,
          config: mcp.config,
          enabled: mcp.isEnabled,
        } as AppAction);
        return;
      }
      dispatch({ type: "CLOSE_MENU" } as AppAction);
    } catch (error) {
      appendSystem(`Menu action failed: ${(error as Error).message}`);
    }
  }, [appendSystem, loadProviderModels, loadSandboxSettings, refreshContextUsage, state.menuKind, state.sessionId, switchSession]);

  const handleMenuDelete = useCallback(async (item: MenuItem | undefined) => {
    if (!item || !state.menuKind) return;

    try {
      if (state.menuKind === "session") {
        const session = item.data as Session;
        await apiRef.current.deleteSession(session.id);
        if (session.id === state.sessionId) {
          dispatch({ type: "RESET_SESSION" } as AppAction);
          applyTerminalTitle("");
          redrawTerminal();
        }
        await loadSessions();
        return;
      }
      if (state.menuKind === "skills") {
        const skill = item.data as Skill;
        if (skill.readOnly) {
          appendSystem("External skills are read-only. Use the web settings page to enable or disable them.");
          return;
        }
        await apiRef.current.deleteSkill(skill.id);
        await loadSkills();
        return;
      }
      if (state.menuKind === "mcp") {
        const mcp = item.data as MCPConfig;
        await apiRef.current.deleteMCPConfig(mcp.id);
        await loadMCPConfigs();
        return;
      }
      if (state.menuKind === "model") {
        const model = item.data as LLMConfig;
        await apiRef.current.deleteLLMConfig(model.id);
        await loadModels();
        await loadDefaultModel();
        return;
      }
      if (state.menuKind === "provider") {
        const provider = item.data as LLMProvider;
        if (pendingProviderDeleteRef.current?.id !== provider.id || Date.now() - pendingProviderDeleteRef.current.at > 5000) {
          pendingProviderDeleteRef.current = { id: provider.id, at: Date.now() };
          appendSystem(`Press D again within 5 seconds to delete ${provider.name} and all its models.`);
          return;
        }
        pendingProviderDeleteRef.current = null;
        await apiRef.current.deleteLLMProvider(provider.id);
        await loadProviders();
        await loadDefaultModel();
        return;
      }
      if (state.menuKind === "provider-model") {
        await apiRef.current.deleteLLMConfig((item.data as LLMConfig).id);
        if (providerMenuRef.current) await loadProviderModels(providerMenuRef.current);
        await loadDefaultModel();
      }
    } catch (error) {
      appendSystem(`Delete failed: ${(error as Error).message}`);
    }
  }, [appendSystem, applyTerminalTitle, redrawTerminal, loadDefaultModel, loadMCPConfigs, loadModels, loadProviderModels, loadProviders, loadSessions, loadSkills, state.menuKind, state.sessionId]);

  const handleMenuAdd = useCallback(() => {
    if (state.menuKind === "mcp") {
      dispatch({ type: "SET_MCP_TEMPLATE_VIEW" } as AppAction);
    } else if (state.menuKind === "model") {
      void loadProviders();
    } else if (state.menuKind === "provider") {
      dispatch({ type: "SET_PROVIDER_EDITOR_VIEW" } as AppAction);
    } else if (state.menuKind === "provider-model" && providerMenuRef.current) {
      dispatch({ type: "SET_MODEL_EDITOR_VIEW", providerId: providerMenuRef.current.id } as AppAction);
    }
  }, [loadProviders, state.menuKind]);

  const handleMenuEdit = useCallback((item: MenuItem | undefined) => {
    if (!item) return;
    if (state.menuKind === "model" || state.menuKind === "provider-model") {
      dispatch({ type: "SET_MODEL_EDITOR", config: item.data as LLMConfig } as AppAction);
      return;
    }
    if (state.menuKind === "provider") {
      dispatch({ type: "SET_PROVIDER_EDITOR", provider: item.data as LLMProvider } as AppAction);
      return;
    }
    if (state.menuKind !== "mcp") return;
    const mcp = item.data as MCPConfig;
    dispatch({
      type: "SET_MCP_EDITOR",
      id: mcp.id,
      name: mcp.name,
      config: mcp.config,
      enabled: mcp.isEnabled,
    } as AppAction);
  }, [state.menuKind]);

  const handleMenuDiscover = useCallback((item: MenuItem | undefined) => {
    const provider = state.menuKind === "provider" ? item?.data as LLMProvider | undefined : providerMenuRef.current;
    if (provider) void loadDiscoveredModels(provider);
  }, [loadDiscoveredModels, state.menuKind]);

  const handleMenuToggle = useCallback(async (item: MenuItem | undefined) => {
    if (state.menuKind !== "mcp" || !item) return;
    const mcp = item.data as MCPConfig;
    try {
      await apiRef.current.updateMCPConfig(mcp.id, {
        name: mcp.name,
        config: mcp.config,
        isEnabled: !mcp.isEnabled,
      });
      await loadMCPConfigs();
    } catch (error) {
      appendSystem(`Failed to toggle MCP config: ${(error as Error).message}`);
    }
  }, [appendSystem, loadMCPConfigs, state.menuKind]);

  const loadMCPToolsForConfig = useCallback(async (mcp: MCPConfig) => {
    dispatch({ type: "SET_MCP_TOOLS_LOADING", loading: true } as AppAction);
    try {
      const result = await apiRef.current.getMCPConfigTools(mcp.id);
      dispatch({ type: "SET_MCP_TOOLS_RESULT", result, error: "" } as AppAction);
    } catch (error) {
      dispatch({ type: "SET_MCP_TOOLS_RESULT", result: null, error: (error as Error).message } as AppAction);
    }
  }, []);

  const handleMenuTools = useCallback((item: MenuItem | undefined) => {
    if (state.menuKind !== "mcp" || !item) return;
    const mcp = item.data as MCPConfig;
    dispatch({ type: "SET_MCP_TOOLS_VIEW", config: mcp } as AppAction);
    void loadMCPToolsForConfig(mcp);
  }, [loadMCPToolsForConfig, state.menuKind]);

  const refreshMCPTools = useCallback(() => {
    if (!state.mcpToolsConfig) return;
    void loadMCPToolsForConfig(state.mcpToolsConfig);
  }, [loadMCPToolsForConfig, state.mcpToolsConfig]);

  const selectMCPTemplate = useCallback((template: MCPTemplate) => {
    dispatch({
      type: "SET_MCP_EDITOR",
      id: "",
      name: "",
      config: template.template,
      enabled: true,
    } as AppAction);
  }, []);

  const saveModelConfig = useCallback(async () => {
    try {
      const payload = {
        name: state.modelEditorName,
        providerId: state.modelEditorProviderId,
        model: state.modelEditorModel,
        contextSize: clampContextSize(state.modelEditorContextSize),
      };
      if (!payload.providerId || !payload.name.trim() || !payload.model.trim()) throw new Error("Name and Model ID are required.");
      if (state.modelEditorId) {
        await apiRef.current.updateLLMConfig(state.modelEditorId, payload);
        appendSystem("Model config updated.");
      } else {
        await apiRef.current.createLLMConfig(payload);
        appendSystem("Model config created.");
      }
      if (providerMenuRef.current) await loadProviderModels(providerMenuRef.current);
      else await loadModels();
    } catch (error) {
      appendSystem(`Failed to save model config: ${(error as Error).message}`);
    }
  }, [
    appendSystem,
    loadModels,
    state.modelEditorName,
    state.modelEditorProviderId,
    state.modelEditorModel,
    state.modelEditorContextSize,
    state.modelEditorId,
    loadProviderModels,
  ]);

  const saveProviderConfig = useCallback(async () => {
    try {
      const payload = {
        name: state.modelEditorName,
        protocol: state.modelEditorProvider,
        baseUrl: state.modelEditorBaseUrl,
        apiKey: state.modelEditorId && state.modelEditorApiKey === "-" ? "" : state.modelEditorApiKey,
        clearApiKey: Boolean(state.modelEditorId && state.modelEditorApiKey === "-"),
      };
      if (!payload.name.trim() || !payload.baseUrl.trim()) throw new Error("Name and Base URL are required.");
      if (state.modelEditorId) await apiRef.current.updateLLMProvider(state.modelEditorId, payload);
      else await apiRef.current.createLLMProvider(payload);
      appendSystem(`Provider ${state.modelEditorId ? "updated" : "created"}.`);
      await loadProviders();
    } catch (error) {
      appendSystem(`Failed to save provider: ${(error as Error).message}`);
    }
  }, [appendSystem, loadProviders, state.modelEditorName, state.modelEditorProvider, state.modelEditorBaseUrl, state.modelEditorApiKey, state.modelEditorId]);

  const saveMCPConfig = useCallback(async () => {
    try {
      if (state.mcpEditorId) {
        await apiRef.current.updateMCPConfig(state.mcpEditorId, {
          name: state.mcpEditorName,
          config: state.mcpEditorConfig,
          isEnabled: state.mcpEditorEnabled,
        });
        appendSystem("MCP config updated.");
      } else {
        await apiRef.current.createMCPConfig({
          name: state.mcpEditorName,
          config: state.mcpEditorConfig,
          isEnabled: state.mcpEditorEnabled,
        });
        appendSystem("MCP config created.");
      }
      await loadMCPConfigs();
    } catch (error) {
      appendSystem(`Failed to save MCP config: ${(error as Error).message}`);
    }
  }, [
    appendSystem,
    loadMCPConfigs,
    state.mcpEditorConfig,
    state.mcpEditorEnabled,
    state.mcpEditorId,
    state.mcpEditorName,
  ]);

  const sendMessage = useCallback(async (content: string) => {
    if (!state.modelId) {
      appendSystem("Please select a model first with /model.");
      return;
    }

    dispatch({ type: "APPEND_ENTRY", entry: { kind: "user", content } } as AppAction);
    dispatch({ type: "STREAM_START" } as AppAction);

    const sendToSocket = (sid: string): boolean => {
      return socketRef.current?.send(content, sid, state.modelId, state.thinkingLevel, state.planMode, state.subagentModelId) || false;
    };

    if (!state.sessionId) {
      try {
        const session = await apiRef.current.createSession();
        dispatch({ type: "SET_SESSION", sessionId: session.id, sessionName: session.name } as AppAction);
        applyTerminalTitle(session.name);
        if (!sendToSocket(session.id)) {
          dispatch({ type: "STREAM_DONE", error: "WebSocket is not connected." } as AppAction);
        }
      } catch (error) {
        dispatch({
          type: "STREAM_DONE",
          error: `Failed to create session: ${(error as Error).message}`,
        } as AppAction);
      }
      return;
    }

    if (!sendToSocket(state.sessionId)) {
      dispatch({ type: "STREAM_DONE", error: "WebSocket is not connected." } as AppAction);
    }
  }, [appendSystem, applyTerminalTitle, state.modelId, state.sessionId, state.planMode, state.thinkingLevel]);

  const handleCommand = useCallback(async (raw: string) => {
    await runCliCommand(raw, {
      newSession: () => {
        dispatch({ type: "RESET_SESSION" } as AppAction);
        applyTerminalTitle("");
        redrawTerminal();
      },
      loadSessions,
      loadModels,
      loadProviders,
      loadSubagentModels,
      toggleApprovalMode,
      toggleThinkingLevel,
      setThinkingLevel,
      loadSkills,
      loadUpdate,
      loadMCPConfigs,
      loadSandboxSettings,
      loadMemory,
      resetMemory,
      openTeamDetail: () => dispatch({ type: "OPEN_TEAM_DETAIL" } as AppAction),
      showHelp,
      togglePlanMode: () => dispatch({ type: "TOGGLE_PLAN_MODE" } as AppAction),
      unknownCommand: (cmd) => appendSystem(`Unknown command: ${cmd}`),
    });
  }, [
    appendSystem,
    applyTerminalTitle,
    redrawTerminal,
    loadMCPConfigs,
    loadModels,
    loadProviders,
    loadMemory,
    loadSandboxSettings,
    loadSessions,
    loadSkills,
    loadUpdate,
    loadSubagentModels,
    resetMemory,
    setThinkingLevel,
    showHelp,
    toggleApprovalMode,
    toggleThinkingLevel,
  ]);

  // Initial clear + default model.
  useEffect(() => {
    if (!forceRedraw(stdout, { clearScrollback: true })) {
      clearScreen();
    }
    applyTerminalTitle("");
    void loadDefaultModel();
    void loadApprovalMode();
    void loadThinkingLevel();
    void checkForUpdatesSilently();
  }, [applyTerminalTitle, loadDefaultModel, loadApprovalMode, loadThinkingLevel, checkForUpdatesSilently, stdout]);

  // Terminal resize.
  useEffect(() => {
    const handleResize = () => setWidth(Math.max(20, stdout?.columns || 80));
    stdout?.on("resize", handleResize);
    return () => { stdout?.off("resize", handleResize); };
  }, [stdout]);

  // Blink timer.
  useEffect(() => {
    if (!state.streaming && !state.assistantWaiting) {
      if (blinkRef.current) {
        clearInterval(blinkRef.current);
        blinkRef.current = null;
      }
      if (!state.blinkOn) {
        dispatch({ type: "BLINK_TOGGLE" } as AppAction);
      }
      return;
    }

    blinkRef.current = setInterval(() => {
      dispatch({ type: "BLINK_TOGGLE" } as AppAction);
      dispatch({ type: "TURN_STATS_TICK" } as AppAction);
    }, 500);

    return () => {
      if (blinkRef.current) {
        clearInterval(blinkRef.current);
        blinkRef.current = null;
      }
    };
  }, [state.streaming, state.assistantWaiting]);

  useCliSocket({
    apiURL,
    cliToken,
    socketRef,
    sessionRef,
    liveAssistantRef,
    planStartedRef,
    preambleShownRef,
    dispatch,
    refreshSessionName,
    applyTerminalTitle,
  });

  useCliKeyboard({
    state,
    dispatch,
    socketRef,
    exit,
    handleMenuSelect,
    handleMenuDelete,
    handleMenuAdd,
    handleMenuEdit,
    handleMenuToggle,
    handleMenuTools,
    handleMenuDiscover,
    loadUpdate,
    applyUpdate,
    loadMemory,
    handleMemoryConsoleSelect,
    saveMemoryConsoleDraft,
    loadMCPConfigs,
    loadModels,
    loadProviders,
    loadProviderModels: () => providerMenuRef.current ? loadProviderModels(providerMenuRef.current) : loadModels(),
    refreshMCPTools,
    saveMCPConfig,
    saveModelConfig,
    saveProviderConfig,
    selectMCPTemplate,
    moveModelProvider,
  });

  const topLevelThinkingActive = state.timeline.some((entry) =>
    entry.kind === "thinking" && entry.thinkingDone !== true
  );

  return (
    <Box flexDirection="column">
      <Banner
        version={state.version}
        modelName={state.modelName}
        cwd={state.cwd}
        approvalMode={state.approvalMode}
        thinkingLevel={state.thinkingLevel}
        updateAvailable={Boolean(state.updateCheck?.updateAvailable)}
      />
      <Text> </Text>
      {state.view !== "team-detail" && (state.timeline.length > 0 || state.streaming) && (
        <>
          <Timeline
            entries={state.timeline}
            blinkOn={state.blinkOn}
            streaming={state.streaming}
            assistantWaiting={state.assistantWaiting}
            liveAssistant={state.liveAssistant}
            maxWidth={width}
            compact={state.compact}
            toolOutputExpanded={state.toolOutputExpanded}
            planGenerating={state.planGenerating}
            planReceived={state.planReceived}
            waitingStatsSuffix={state.streaming ? formatWaitingStatsSuffix({
              elapsedMs: state.turnElapsedMs,
              tokenEstimate: state.turnTokenEstimate,
              thoughtDurationMs: state.turnThoughtDurationMs,
              thinkingActive: topLevelThinkingActive,
            }) : ""}
            runtimeTodos={state.runtimeTodos}
          />
          <Text> </Text>
        </>
      )}
      {state.contextUsage && (
        <Text color={state.contextUsage.usedPercent >= 90 ? "red" : state.contextUsage.usedPercent >= 70 ? "yellow" : "green"}>
          {formatContextUsageStatus(state.contextUsage, width)}
        </Text>
      )}
      <Text color="white">{border}</Text>

      {state.view === "chat" && (
        <Box>
          <Text bold color={state.streaming ? "gray" : "white"}>
            ❯{" "}
          </Text>
          <TextInput
            key={state.inputKey}
            value={state.inputValue}
            onChange={(value) => dispatch({ type: "SET_INPUT", value } as AppAction)}
            onSubmit={(rawValue) => {
              const value = rawValue.trim();
              if (!value) return;
              dispatch({ type: "SET_INPUT", value: "" } as AppAction);

              // Kept behind the display flag so hidden thinking data remains recoverable later.
              const num = parseInt(value, 10);
              if (SHOW_CLI_THINKING && !isNaN(num) && String(num) === value && num > 0) {
                const thinkingEntries = state.timeline.filter((e) => e.kind === "thinking");
                if (num <= thinkingEntries.length) {
                  dispatch({
                    type: "VIEW_THINKING_DETAIL",
                    content: thinkingEntries[num - 1].content || "(empty)",
                  } as AppAction);
                  return;
                }
              }

              if (isCommand(value)) {
                void handleCommand(value);
              } else {
                void sendMessage(value);
              }
            }}
            onTab={() => {
              const completed = completeCommand(state.inputValue, selectedCommandHintIndex);
              if (!completed) return undefined;
              const next = `${completed} `;
              dispatch({
                type: "SET_INPUT_WITH_KEY",
                value: next,
              } as AppAction);
              return next;
            }}
            onEscape={() => {
              if (state.inputValue) {
                dispatch({ type: "SET_INPUT", value: "" } as AppAction);
              }
            }}
            onBeforeInput={(_, key) => {
              if (!hasCommandHints) return false;
              if (key.upArrow || key.downArrow) {
                setCommandHintCursor((current) => (
                  moveCommandHintCursor(current, key.upArrow ? -1 : 1, commandHints.length)
                ));
                return true;
              }
              if (key.return) {
                const completed = completeCommand(state.inputValue, selectedCommandHintIndex);
                if (!completed) return false;
                dispatch({
                  type: "SET_INPUT_WITH_KEY",
                  value: `${completed} `,
                } as AppAction);
                return true;
              }
              return false;
            }}
            onUnhandledInput={(input, key) => {
              if (state.view !== "chat" || state.streaming) return;
              handleChatShortcut(input, key, dispatch);
            }}
            focus={state.view === "chat" && !state.streaming}
            columns={Math.max(20, width - 3)}
          />
        </Box>
      )}

      {state.view === "menu" && state.menuKind && (
        <MenuView
          title={state.menuTitle}
          items={state.menuItems}
          cursor={state.menuCursor}
          hint={state.menuHint}
          maxVisibleItems={state.menuKind === "session" ? MENU_VISIBLE_LIMIT : undefined}
        />
      )}

      {state.view === "approval" && (
        <ApprovalView
          toolName={state.approvalToolName}
          command={state.approvalCommand}
          params={state.approvalParams}
          items={state.pendingApprovals}
          approvalReviewItems={state.approvalReviewItems}
          cursor={state.approvalCursor}
          columns={width}
        />
      )}

      {state.view === "plan-confirm" && (
        <PlanConfirmView
          cursor={state.planConfirmCursor}
          feedback={state.planModifyInput}
          feedbackKey={state.planModifyInputKey}
          onFeedbackChange={(value) => dispatch({ type: "SET_PLAN_MODIFY_INPUT", value } as AppAction)}
          onFeedbackSubmit={(rawValue) => {
            const feedback = rawValue.trim();
            if (!feedback) return;
            dispatch({ type: "APPEND_ENTRY", entry: { kind: "user", content: feedback } } as AppAction);
            socketRef.current?.sendPlanModify(
              state.pendingPlanId, state.sessionId, state.modelId,
              feedback, state.thinkingLevel,
            );
            dispatch({ type: "CLEAR_PLAN_CONFIRMATION" } as AppAction);
          }}
          onEscape={() => {
            socketRef.current?.sendPlanReject(
              state.pendingPlanId, state.sessionId,
            );
            dispatch({ type: "CLEAR_PLAN_CONFIRMATION" } as AppAction);
          }}
          columns={width}
        />
      )}

            {state.view === "question-answer" && (
        <QuestionAnswerView
          questions={state.qaQuestions}
          currentIndex={state.qaCurrentIndex}
          answers={state.qaAnswers}
          step={state.qaStep}
          cursor={state.qaCursor}
          customInput={state.qaCustomInput}
          onCustomInputChange={(value) => dispatch({ type: "QA_SET_CUSTOM_INPUT", value } as AppAction)}
          onCustomInputSubmit={(value) => {
            const trimmed = value.trim();
            if (!trimmed) return;
            dispatch({ type: "QA_SUBMIT_CUSTOM", value: trimmed } as AppAction);
            dispatch(state.qaCurrentIndex < state.qaQuestions.length - 1
              ? { type: "QA_NEXT_QUESTION" } as AppAction
              : { type: "QA_STEP_CONFIRM" } as AppAction);
          }}
          onEscape={() => {
            const cancelAnswers = JSON.stringify(
              state.qaQuestions.map((q: { id: string }) => ({ questionId: q.id, selectedOption: -2, customAnswer: "" })),
            );
            socketRef.current?.sendToolApproval(state.qaToolCallId, true, cancelAnswers);
            dispatch({ type: "CLEAR_QA" } as AppAction);
          }}
          columns={width}
        />
      )}

      {state.view === "thinking-detail" && (
        <Box flexDirection="column">
          <Text bold color="magenta">{"Thinking Detail"}</Text>
          <Text color="gray">Press Esc to return</Text>
          <Text> </Text>
          <Box flexDirection="column">
            {state.thinkingDetailContent.split("\n").map((line, i) => (
              <Text key={i} dimColor>{line}</Text>
            ))}
          </Box>
        </Box>
      )}

      {state.view === "update" && (
        <UpdateView
          check={state.updateCheck}
          job={state.updateJob}
          loading={state.updateLoading}
          applying={state.updateApplying}
          confirming={state.updateConfirming}
          columns={width}
        />
      )}

      {state.view === "memory-console" && (
        <MemoryConsoleView
          snapshot={state.memorySnapshot}
          loading={state.memoryLoading}
          cursor={state.memoryCursor}
          mode={state.memoryMode}
          editingField={state.memoryEditingField}
          draft={state.memoryDraft}
          viewTarget={state.memoryViewTarget}
          message={state.memoryMessage}
          columns={width}
        />
      )}

      {state.view === "team-detail" && (
        <TeamView
          runs={teamRuns}
          entries={state.timeline}
          teamCursor={state.teamRunCursor}
          memberCursor={state.teamMemberCursor}
          columns={width}
        />
      )}

      {state.view === "mcp-editor" && (
        <MCPEditor
          id={state.mcpEditorId}
          name={state.mcpEditorName}
          config={state.mcpEditorConfig}
          enabled={state.mcpEditorEnabled}
          focusName={state.mcpEditorFocusName}
          onNameChange={(name) =>
            dispatch({ type: "SET_MCP_EDITOR_NAME", name } as AppAction)
          }
          onConfigChange={(config) =>
            dispatch({ type: "SET_MCP_EDITOR_CONFIG", config } as AppAction)
          }
          onToggleEnabled={() => dispatch({ type: "TOGGLE_MCP_EDITOR_ENABLED" } as AppAction)}
          onToggleFocus={() => dispatch({ type: "TOGGLE_MCP_EDITOR_FOCUS" } as AppAction)}
          onSave={() => {
            void saveMCPConfig();
          }}
          onBack={() => {
            void loadMCPConfigs();
          }}
        />
      )}

      {state.view === "mcp-template" && (
        <MCPTemplatePicker cursor={state.mcpTemplateCursor} />
      )}

      {state.view === "mcp-tools" && (
        <MCPToolsView
          config={state.mcpToolsConfig}
          result={state.mcpToolsResult}
          loading={state.mcpToolsLoading}
          error={state.mcpToolsError}
        />
      )}

      {state.view === "model-editor" && (
        <ModelEditor
          name={state.modelEditorName}
          model={state.modelEditorModel}
          contextSize={state.modelEditorContextSize}
          focusIndex={state.modelEditorFocusIndex}
          onNameChange={(name) => dispatch({ type: "SET_MODEL_EDITOR_NAME", name } as AppAction)}
          onModelChange={(model) => dispatch({ type: "SET_MODEL_EDITOR_MODEL", model } as AppAction)}
          onContextSizeChange={(contextSize) => dispatch({ type: "SET_MODEL_EDITOR_CONTEXT_SIZE", contextSize } as AppAction)}
        />
      )}

      {state.view === "provider-editor" && (
        <ProviderEditor
          name={state.modelEditorName}
          protocol={state.modelEditorProvider}
          baseUrl={state.modelEditorBaseUrl}
          apiKey={state.modelEditorApiKey}
          focusIndex={state.modelEditorFocusIndex}
          protocolSelect={state.modelEditorProviderSelect}
          protocolCursor={Math.max(0, MODEL_PROVIDER_ORDER.indexOf(state.modelEditorProvider))}
          onNameChange={(name) => dispatch({ type: "SET_MODEL_EDITOR_NAME", name } as AppAction)}
          onBaseUrlChange={(url) => dispatch({ type: "SET_MODEL_EDITOR_BASE_URL", baseUrl: url } as AppAction)}
          onApiKeyChange={(key) => dispatch({ type: "SET_MODEL_EDITOR_API_KEY", apiKey: key } as AppAction)}
        />
      )}

      <Text color="white">{border}</Text>

      {state.view === "chat" && state.streaming && (
        <Text color={CLI_HINT_COLOR}>
          Generating response | Esc to cancel
        </Text>
      )}

      {hasCommandHints && (
        <Box flexDirection="column">
          <CommandHints input={state.inputValue} selectedIndex={selectedCommandHintIndex} />
          <Text color={CLI_HINT_COLOR}>
            ↑↓ to select | Enter/Tab to fill | Esc to clear
          </Text>
        </Box>
      )}

      {state.view === "chat" && !state.streaming && !hasCommandHints && (
        <Box justifyContent="space-between">
          <Text color={CLI_HINT_COLOR}>
            {getChatFooterHint(state.planMode, state.approvalMode)}
          </Text>
          <Box>
            {state.planMode && <Text color={CLI_ACCENT_COLOR} bold>◆ Plan </Text>}
            {state.approvalMode === "auto_review" && <Text color="#eab308" bold>◆ Auto Review </Text>}
            {state.approvalMode === "auto" && <Text color="#eab308" bold>◆ Auto </Text>}
          </Box>
        </Box>
      )}

      {state.view === "approval" && (
        <Text color={CLI_HINT_COLOR}>
          ↑/↓ switch | Y approve | N reject | A approve all | R reject all
        </Text>
      )}

      {state.view === "plan-confirm" && (
        <Text color={CLI_HINT_COLOR}>
          Arrow keys to navigate | Enter to select | Esc to cancel
        </Text>
      )}

      {state.view === "team-detail" && (
        <Text color={CLI_HINT_COLOR}>
          ←/→ Team | ↑/↓ member | Esc back
        </Text>
      )}

      {state.view === "mcp-editor" && (
        <Text color={CLI_HINT_COLOR}>
          Tab to switch field | Ctrl+S to save | Ctrl+E to toggle | Esc to go back
        </Text>
      )}

      {state.view === "mcp-template" && (
        <Text color={CLI_HINT_COLOR}>
          Arrow keys to navigate | Enter to select | Esc to cancel
        </Text>
      )}

      {state.view === "model-editor" && (
        <Text color={CLI_HINT_COLOR}>
          Tab to switch field | Ctrl+S to save | Esc to go back
        </Text>
      )}

      {state.view === "provider-editor" && (
        <Text color={CLI_HINT_COLOR}>
          Tab to switch field | Ctrl+S to save | Esc to go back{state.modelEditorFocusIndex === 1 ? " | Enter to change protocol" : ""}
        </Text>
      )}
    </Box>
  );
}
