import { randomUUID } from "node:crypto";
import { runPrivacyCommand } from "./utils/privacy.js";
﻿/**
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
import { CheckpointView } from "./components/CheckpointView.js";
import { CommandHints } from "./components/CommandHints.js";
import { ContextView } from "./components/ContextView.js";
import { MCPEditor } from "./components/MCPEditor.js";
import { MCPTemplatePicker } from "./components/MCPTemplatePicker.js";
import { MCPToolsView } from "./components/MCPToolsView.js";
import { CLI_HINT_COLOR, MenuView, MENU_VISIBLE_LIMIT, getMenuViewport } from "./components/MenuView.js";
import MemoryConsoleView from "./components/MemoryConsoleView.js";
import CooperativeView from "./components/CooperativeView.js";
import type { CooperativeSnapshot } from "./types/cooperative.js";
import TeamView from "./components/TeamView.js";
import { ModelEditor } from "./components/ModelEditor.js";
import { ProviderEditor } from "./components/ProviderEditor.js";
import { TextInput } from "./components/TextInput.js";
import { Timeline } from "./components/Timeline.js";
import { UpdateView } from "./components/UpdateView.js";
import { UsageView } from "./components/UsageView.js";
import { getChatFooterHint, handleChatShortcut, runCliCommand } from "./controllers/commands.js";
import { applySessionResponse, captureSessionRequest, type SessionSelection } from "./controllers/sessionRequests.js";
import { createMemoryConsoleController } from "./controllers/memoryConsole.js";
import { createUpdateController } from "./controllers/updateController.js";
import { createWorkspaceConsoleController } from "./controllers/workspaceConsole.js";
import { useCliKeyboard } from "./hooks/useCliKeyboard.js";
import { clampContextSize, formatContextSize, formatContextTokenCount, formatContextUsageStatus } from "./utils/contextSize.js";
import { useCliSocket } from "./hooks/useCliSocket.js";
import { reducer, createInitialState } from "./reducer.js";
import { completeCommand, getCommandMeta, isCommand, matchCommandHints, moveCommandHintCursor, resolveCommandHintEnter } from "./utils/commands.js";
import { formatTimestamp, formatWaitingStatsSuffix } from "./utils/format.js";
import { getAgentTeamRuns } from "./utils/agentTeam.js";
import { mapHistoryMessages } from "./utils/history.js";
import { buildSandboxMenuItems, type SandboxMenuAction } from "./utils/sandboxSettings.js";
import { CLI_ACCENT_COLOR, clearScreen, setTerminalTitle, terminalWorkspaceLayout } from "./utils/terminal.js";
import { CLI_THEME } from "./utils/theme.js";
import { SHOW_CLI_THINKING } from "./utils/timelineFormat.js";
import { getOutputBudgetStatus } from "./utils/usage.js";
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
  PersistedBackgroundTask,
  Session,
  Skill,
} from "./types.js";
import { SUPPORTED_COMMANDS, THINKING_LEVELS } from "./types.js";

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
  const [terminalRows, setTerminalRows] = React.useState(() => stdout?.rows || 24);
  const [cooperativeLoading, setCooperativeLoading] = React.useState(false);
  const [cooperativeError, setCooperativeError] = React.useState("");
  const [cooperativeView, setCooperativeView] = React.useState(false);
  const [cooperativeTab, setCooperativeTab] = React.useState(0);
  const [cooperativeSnapshot, setCooperativeSnapshot] = React.useState<CooperativeSnapshot|null>(null);
  const [commandHintCursor, setCommandHintCursor] = React.useState(0);
  const [backgroundTaskCount, setBackgroundTaskCount] = React.useState(0);
  const [contextCompacting, setContextCompacting] = React.useState(false);
  const [outputBudgetTokens, setOutputBudgetTokens] = React.useState<number>(0);
  const border = "─".repeat(width);

  const [state, dispatch] = useReducer(
    reducer,
    createInitialState(apiURL, cliToken, process.cwd(), version),
  );
  const stateRef = useRef(state);
  stateRef.current = state;

  const apiRef = useRef(new APIClient(apiURL, cliToken));
  const providerMenuRef = useRef<LLMProvider | null>(null);
  const pendingProviderDeleteRef = useRef<{ id: string; at: number } | null>(null);
  const socketRef = useRef<CLISocket | null>(null);
  const blinkRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sessionRef = useRef<SessionSelection>({ id: "", name: "", generation: 0 });
  const modelIdRef = useRef(state.modelId);
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

  const invalidateSessionRequests = useCallback(() => {
    sessionRef.current.generation++;
  }, []);

  const setCurrentSession = useCallback((id: string, name = "") => {
    if (id !== sessionRef.current.id) setBackgroundTaskCount(0);
    sessionRef.current = { id, name, generation: sessionRef.current.generation + 1 };
    socketRef.current?.watchSession(id);
  }, []);

  const refreshContextUsage = useCallback(async (sessionId: string, modelId: string) => {
    if (!sessionId || !modelId) return;
    await applySessionResponse(
      captureSessionRequest(sessionRef, sessionId),
      () => apiRef.current.getContextUsage(sessionId, modelId),
      (usage) => dispatch({ type: "CONTEXT_USAGE", usage }),
    );
  }, []);

  const applyTerminalTitle = useCallback((sessionName = "") => {
    setTerminalTitle(sessionName.trim() || "SlimeBot CLI");
  }, []);

  const {
    loadMemory,
    resetMemory,
    handleMemoryConsoleSelect,
    saveMemoryConsoleDraft,
  } = React.useMemo(() => createMemoryConsoleController({
    api: apiRef.current,
    dispatch,
    appendSystem,
    getState: () => stateRef.current,
  }), [appendSystem]);

  const {
    loadUpdate,
    checkForUpdatesSilently,
    applyUpdate,
    refreshUpdateJob,
  } = React.useMemo(() => createUpdateController({
    api: apiRef.current,
    dispatch,
    appendSystem,
    getState: () => stateRef.current,
  }), [appendSystem]);

  const { handleCheckpointCommand, handleUsageCommand } = React.useMemo(() => createWorkspaceConsoleController({
    api: apiRef.current,
    dispatch,
    appendSystem,
    getState: () => stateRef.current,
    getSessionId: () => sessionRef.current.id,
    getOutputBudgetTokens: () => outputBudgetTokens,
    setOutputBudgetTokens,
    refreshContextUsage,
  }), [appendSystem, outputBudgetTokens, refreshContextUsage]);


  useEffect(() => {
    modelIdRef.current = state.modelId;
  }, [state.modelId]);

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
    if (!sessionId) return;
    await applySessionResponse(
      captureSessionRequest(sessionRef, sessionId),
      () => apiRef.current.listSessions(200, 0),
      ({ sessions }) => {
        const session = sessions.find((s) => s.id === sessionId);
        if (!session) return;
        sessionRef.current.name = session.name;
        dispatch({ type: "SET_SESSION_NAME", sessionName: session.name });
        applyTerminalTitle(session.name);
      },
    );
  }, [applyTerminalTitle]);

  const loadDefaultModel = useCallback(async () => {
    try {
      const settings = await apiRef.current.getSettings();
      setOutputBudgetTokens(Number(settings.sessionOutputTokenBudget) || 0);
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
      hint: "↑↓ navigate · Enter select · Esc cancel",
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
        hint: "↑↓ navigate · Enter switch · D delete · Esc close",
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
        hint: "Enter switch · A providers · E edit · D delete · Esc close",
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
        hint: "Enter models · A add · E edit · D delete · R discover · Esc back",
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
        items: models.map(model => ({ title: model.name, desc: `${model.model} · ${formatContextSize(model.contextSize || 1_000_000)} ${model.contextSizeSource === "detected" ? "(detected)" : model.contextSizeSource === "fallback" ? "(estimate)" : "(custom)"}`, data: model })),
        hint: "Enter edit · A add manually · D delete · R discover · Esc providers",
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
        items: found.map(model => ({ title: model.name, desc: existing.has(model.id) ? `${model.id} · already added` : `${model.id} · ${model.contextSize ? formatContextSize(model.contextSize) : "context unknown"}`, data: { ...model, added: existing.has(model.id) } })),
        hint: "Enter add model · Esc provider models",
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
        hint: "↑↓ navigate · Enter select · Esc close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load subagent models: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadSkills = useCallback(async () => {
    try {
      const skills = await apiRef.current.listSkills();
      const items: MenuItem[] = skills.map((s: Skill) => {
        const isEnabled = s.enabled !== false;
        const statusMark = isEnabled ? "● ON" : "○ OFF";
        return {
          title: s.name,
          desc: `${statusMark} · ${s.sourceLabel || "SlimeBot"}${s.readOnly ? " · read-only" : ""} · ${s.description}`,
          data: s,
        };
      });
      dispatch({
        type: "SET_MENU",
        kind: "skills",
        title: "Skills Menu",
        items,
        hint: "↑↓ navigate · Space toggle · D delete · Esc close",
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
        hint: "↑↓ navigate · Enter/E edit · T tools · A add · Space toggle · D delete · Esc close",
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
        hint: "↑↓ navigate · Enter apply · Esc close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load sandbox settings: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const privacy = useCallback(async (input: string) => {
    try { appendSystem(await runPrivacyCommand(apiRef.current, input, sessionRef.current.id)); }
    catch (error) { appendSystem(`Privacy: ${(error as Error).message}`); }
  }, [appendSystem]);

  const loadBackgroundTasksMenu = useCallback(async () => {
    const sid = sessionRef.current.id;
    if (!sid) {
      appendSystem("Create or open a session first.");
      return;
    }
    try {
      const tasks = await apiRef.current.listBackgroundTasks(sid);
      const activeCount = tasks.filter((t) => t.status === "running" || t.status === "queued" || t.status === "waking").length;
      setBackgroundTaskCount(activeCount);
      const items: MenuItem[] = tasks.map((task) => {
        const cmdPreview = (task.command || task.processId || task.id).replace(/\s+/g, " ").trim();
        const exitPart = task.status === "completed" || task.status === "failed" ? ` · exit ${task.exitCode}` : "";
        return {
          title: `[${task.status}] ${cmdPreview}`,
          desc: `${task.processId || task.id.slice(0, 8)}${exitPart} · ${formatTimestamp(task.finishedAt || task.startedAt)}${task.error ? ` · ${task.error}` : ""}`,
          data: task,
        };
      });
      dispatch({
        type: "SET_MENU",
        kind: "background",
        title: "Background Tasks",
        items,
        hint: "Enter smart action · C continue · R retry · S stop · Esc close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load background tasks: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const handleBackgroundAction = useCallback(async (
    item: MenuItem | undefined,
    action: "continue" | "retry" | "stop",
  ) => {
    if (!item) return;
    const task = item.data as PersistedBackgroundTask;
    try {
      await apiRef.current.actOnBackgroundTask(task.id, action);
      appendSystem(`Background task ${task.processId || task.id.slice(0, 8)}: ${action} requested.`);
      await loadBackgroundTasksMenu();
    } catch (error) {
      appendSystem(`Background action failed: ${(error as Error).message}`);
    }
  }, [appendSystem, loadBackgroundTasksMenu]);

  const handleCompactContext = useCallback(async () => {
    const sid = sessionRef.current.id;
    if (!sid) {
      appendSystem("Create or open a session first.");
      return;
    }
    if (!state.modelId) {
      appendSystem("Please select a model first with /model.");
      return;
    }
    if (state.streaming || contextCompacting) {
      appendSystem("Cannot compact context while a turn or compaction is already running.");
      return;
    }
    setContextCompacting(true);
    try {
      const result = await apiRef.current.compactContext(sid, state.modelId);
      if (result.usage) {
        dispatch({ type: "CONTEXT_USAGE", usage: result.usage } as AppAction);
      } else {
        await refreshContextUsage(sid, state.modelId);
      }
      if (result.compacted) {
        const before = result.usage?.compactionBeforeTokens;
        const after = result.usage?.compactionAfterTokens;
        const summary = before !== undefined && after !== undefined
          ? `Context compacted: ${formatContextTokenCount(before)} → ${formatContextTokenCount(after)} (saved ${formatContextTokenCount(Math.max(0, before - after))}).`
          : "Context compacted successfully.";
        appendSystem(summary);
      } else {
        appendSystem(result.reason ? `Context compaction skipped: ${result.reason}` : "Context does not need compaction right now.");
      }
    } catch (error) {
      appendSystem(`Context compaction failed: ${(error as Error).message}`);
    } finally {
      setContextCompacting(false);
    }
  }, [appendSystem, contextCompacting, refreshContextUsage, state.modelId, state.streaming]);

  const handleShowContext = useCallback(async () => {
    if (sessionRef.current.id && state.modelId) {
      await refreshContextUsage(sessionRef.current.id, state.modelId);
    }
    dispatch({ type: "SET_VIEW", view: "context" } as AppAction);
  }, [refreshContextUsage, state.modelId]);

  // Checkpoint and usage orchestration lives in workspaceConsole.ts.

  // Memory console orchestration lives in memoryConsole.ts.

  // Update orchestration lives in updateController.ts.

  useEffect(() => {
    if (!state.updateApplying) return;
    let cancelled = false;
    const poll = () => refreshUpdateJob(() => cancelled);
    const timer = setInterval(() => { void poll(); }, 1500);
    void poll();
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [refreshUpdateJob, state.updateApplying]);

  const showHelp = useCallback(() => {
    const categoryLabels: Record<string, string> = {
      session: "Session",
      model: "Model",
      workspace: "Workspace",
      safety: "Safety",
      system: "System",
    };
    const items: MenuItem[] = [
      ...SUPPORTED_COMMANDS.map((cmd) => ({
        title: cmd.command,
        desc: `[${categoryLabels[cmd.category] || cmd.category}] ${cmd.description}`,
        data: cmd.command,
      })),
      {
        title: "Shortcuts",
        desc: "Shift+Tab plan mode · Ctrl+O tool output · Ctrl+K compact markdown · Esc cancel/back",
        data: null,
      },
    ];
    dispatch({
      type: "SET_MENU",
      kind: "help",
      title: "Help · Commands & Shortcuts",
      items,
      hint: "↑↓ navigate · Enter run · Esc close",
    } as AppAction);
  }, []);

  const syncSessionHistory = useCallback(async (sessionId: string, afterReconnect = false) => {
    await applySessionResponse(
      captureSessionRequest(sessionRef, sessionId),
      () => apiRef.current.getSessionMessages(sessionId),
      (history) => {
        const entries = mapHistoryMessages(
          history.messages,
          history.toolCallsByAssistantMessageId || {},
          history.thinkingByAssistantMessageId || {},
          history.teamRuns || [],
          history.teamMemberRuns || [],
        );
        dispatch({ type: "LOAD_HISTORY", entries, afterReconnect });
        void refreshContextUsage(sessionId, modelIdRef.current);
      },
      (error) => appendSystem(`Failed to load session history: ${(error as Error).message}`),
    );
  }, [appendSystem, refreshContextUsage]);

  const resyncSessionHistory = useCallback((sessionId: string) => {
    invalidateSessionRequests();
    void syncSessionHistory(sessionId, true);
  }, [invalidateSessionRequests, syncSessionHistory]);

  const switchSession = useCallback(async (session: Session) => {
    const changed = session.id !== sessionRef.current.id;
    setCurrentSession(session.id, session.name);
    dispatch({ type: "SET_SESSION", sessionId: session.id, sessionName: session.name } as AppAction);
    if (changed) dispatch({ type: "LOAD_HISTORY", entries: [] });
    dispatch({ type: "CLOSE_MENU" } as AppAction);
    applyTerminalTitle(session.name);
    redrawTerminal();
    await syncSessionHistory(session.id);
  }, [applyTerminalTitle, redrawTerminal, setCurrentSession, syncSessionHistory]);

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
        await apiRef.current.createLLMConfig({ providerId: provider.id, name: model.name, model: model.id, contextSize: model.contextSize || 0, contextSizeSource: model.contextSize ? "detected" : "fallback" });
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
      if (state.menuKind === "background") {
        const task = item.data as PersistedBackgroundTask;
        const smartAction: "stop" | "retry" | "continue" =
          task.status === "running" || task.status === "queued" || task.status === "waking"
            ? "stop"
            : task.status === "interrupted" || task.status === "failed"
              ? "retry"
              : "continue";
        await handleBackgroundAction(item, smartAction);
        return;
      }
      if (state.menuKind === "help" && typeof item.data === "string" && item.data.startsWith("/")) {
        dispatch({ type: "CLOSE_MENU" } as AppAction);
        const targetCmd = item.data;
        if (targetCmd !== "/help") {
          setImmediate(() => { void handleCommandRef.current?.(targetCmd); });
        }
        return;
      }
      dispatch({ type: "CLOSE_MENU" } as AppAction);
    } catch (error) {
      appendSystem(`Menu action failed: ${(error as Error).message}`);
    }
  }, [appendSystem, handleBackgroundAction, loadProviderModels, loadSandboxSettings, refreshContextUsage, state.menuKind, state.sessionId, switchSession]);

  const handleMenuDelete = useCallback(async (item: MenuItem | undefined) => {
    if (!item || !state.menuKind) return;

    try {
      if (state.menuKind === "session") {
        const session = item.data as Session;
        await apiRef.current.deleteSession(session.id);
        if (session.id === sessionRef.current.id) {
          setCurrentSession("");
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
          appendSystem("External skills are read-only and cannot be deleted. Press Space to enable or disable them.");
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
  }, [appendSystem, applyTerminalTitle, redrawTerminal, loadDefaultModel, loadMCPConfigs, loadModels, loadProviderModels, loadProviders, loadSessions, loadSkills, setCurrentSession, state.menuKind]);

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
    if (!item) return;
    if (state.menuKind === "skills") {
      const skill = item.data as Skill;
      try {
        const nextEnabled = skill.enabled === false;
        await apiRef.current.setSkillEnabled(skill.id, nextEnabled);
        appendSystem(`Skill ${skill.name} ${nextEnabled ? "enabled" : "disabled"}.`);
        await loadSkills();
      } catch (error) {
        appendSystem(`Failed to toggle skill: ${(error as Error).message}`);
      }
      return;
    }
    if (state.menuKind !== "mcp") return;
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
  }, [appendSystem, loadMCPConfigs, loadSkills, state.menuKind]);

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
        contextSize: state.modelEditorContextSizeSource === "manual" ? clampContextSize(state.modelEditorContextSize) : state.modelEditorContextSizeSource === "detected" ? Number(state.modelEditorContextSize) : 0,
        contextSizeSource: state.modelEditorContextSizeSource === "fallback" ? "auto" as const : state.modelEditorContextSizeSource,
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
    state.modelEditorContextSizeSource,
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
    const sessionId = sessionRef.current.id;
    if (state.streaming || state.assistantWaiting) {
      if (!socketRef.current?.sendSteering(content, sessionId, randomUUID())) { dispatch({ type: "SET_INPUT", value: content } as AppAction); appendSystem("The active turn is not ready for changes. Your input has been kept."); }
      return;
    }

    if (!state.modelId) {
      appendSystem("Please select a model first with /model.");
      return;
    }

    invalidateSessionRequests();
    dispatch({ type: "APPEND_ENTRY", entry: { kind: "user", content } } as AppAction);
    dispatch({ type: "STREAM_START" } as AppAction);

    const sendToSocket = (sid: string): boolean => {
      return socketRef.current?.send(content, sid, state.modelId, state.thinkingLevel, state.planMode, state.subagentModelId) || false;
    };

    if (!sessionId) {
      const isCurrent = captureSessionRequest(sessionRef, "");
      try {
        const session = await apiRef.current.createSession(undefined, state.cwd);
        if (!isCurrent()) return;
        setCurrentSession(session.id, session.name);
        dispatch({ type: "SET_SESSION", sessionId: session.id, sessionName: session.name } as AppAction);
        applyTerminalTitle(session.name);
        if (!sendToSocket(session.id)) {
          dispatch({ type: "STREAM_DONE", error: "WebSocket is not connected." } as AppAction);
        }
      } catch (error) {
        if (!isCurrent()) return;
        dispatch({
          type: "STREAM_DONE",
          error: `Failed to create session: ${(error as Error).message}`,
        } as AppAction);
      }
      return;
    }

    if (!sendToSocket(sessionId)) {
      dispatch({ type: "STREAM_DONE", error: "WebSocket is not connected." } as AppAction);
    }
  }, [appendSystem, applyTerminalTitle, invalidateSessionRequests, setCurrentSession, state.cwd, state.modelId, state.planMode, state.subagentModelId, state.thinkingLevel, state.streaming, state.assistantWaiting]);

  const manageAgents = useCallback(async (command: string, inline = false) => {
    if (!state.sessionId) { appendSystem("Create or open a session first."); return; }
    const root = state.sessionId;
    try {
      const parts = command.split(/\s+/);
      const [action, id, operation] = parts;
      if (action === "send" && id) {
        await apiRef.current.cooperativeAction(id, {action: "message", message: parts.slice(2).join(" "), delivery: "queue", clientMessageId: crypto.randomUUID()});
        appendSystem("Follow-up accepted.");
      } else if (action === "stop") {
        if (!id) throw new Error("Use stop <agent-id|all>");
        if (id === "all") await apiRef.current.cooperativeStop(root);
        else if (id) await apiRef.current.cooperativeAction(id, {action: "interrupt", scope: "subtree"});
      } else if (action === "artifact" && id) {
        const result = await apiRef.current.cooperativeArtifact(id, root, operation || "inspect", parts.slice(3).join(" "));
        if (result.diff) appendSystem(result.diff);
      } else if (action === "approve" && id) {
        if (!["yes", "no"].includes(operation)) throw new Error("Use /agents approve <id> yes|no");
        await apiRef.current.cooperativeApprove(id, operation === "yes");
      } else if (action === "result" && id) {
        const turns = await apiRef.current.cooperativeTurns(id, operation);
        const turn = operation ? turns.find(t => t.id === operation) : turns[0];
        if (!turn) throw new Error("Agent report not found");
        appendSystem(`${turn.status} · ${turn.id}\n${turn.answer || turn.error || "Awaiting report"}`);
        setCooperativeView(false);
        dispatch({type:"SET_VIEW",view:"chat"} as AppAction);
        return;
      } else if (action === "task") {
        const snapshot = await apiRef.current.cooperativeSnapshot(root);
        if (id === "create") await apiRef.current.cooperativeTask(root, {action:"create",title:parts.slice(2).join(" ")});
        else {
          const task=snapshot.tasks.find(t=>t.id===operation);if (!task) throw new Error("Task not found");
          await apiRef.current.cooperativeTask(root,{action:id,taskId:task.id,expectedRevision:task.revision,ownerId:parts[3]});
        }
      } else if (command && !["tasks", "artifacts", "result"].includes(action)) {
        throw new Error("Use /agents [send|stop|task|artifact|approve]");
      }
      setCooperativeSnapshot(await apiRef.current.cooperativeSnapshot(root));
      if (action === "tasks" || action === "task") setCooperativeTab(1);
      else if (action === "artifacts" || action === "artifact") setCooperativeTab(2);
      else if (action === "approve") setCooperativeTab(3);
      else if (command || !cooperativeView) setCooperativeTab(0);
      setCooperativeError("");
      setCooperativeView(true);
      dispatch({type:"OPEN_TEAM_DETAIL"} as AppAction);
    } catch (error) {
      if (inline) throw error;
      appendSystem(`Collaboration: ${(error as Error).message}`);
    }
  }, [state.sessionId, appendSystem, cooperativeView]);

  const refreshAgents = useCallback(async () => {
    const root = state.sessionId;
    if (!root || cooperativeLoading) return;
    setCooperativeLoading(true);
    try {
      const snapshot = await apiRef.current.cooperativeSnapshot(root);
      if (sessionRef.current.id === root) { setCooperativeSnapshot(snapshot); setCooperativeError(""); }
    } catch (error) { setCooperativeError(`Refresh failed: ${(error as Error).message}`); }
    finally { setCooperativeLoading(false); }
  }, [state.sessionId, cooperativeLoading]);

  useEffect(() => {
    if (!cooperativeView || state.view !== "team-detail" || !state.sessionId) return;
    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const refresh = async () => {
      try {
        const snapshot = await apiRef.current.cooperativeSnapshot(state.sessionId!);
        if (!disposed) { setCooperativeSnapshot(snapshot); setCooperativeError(""); }
      } catch (error) { if (!disposed) setCooperativeError(`Connection unavailable: ${(error as Error).message}`); }
      if (!disposed) timer = setTimeout(() => void refresh(), 2000);
    };
    void refresh();
    return () => { disposed = true; clearTimeout(timer); };
  }, [cooperativeView, state.view, state.sessionId]);

  const handleCommandRef = useRef<((raw: string) => Promise<void>) | null>(null);

  const handleCommand = useCallback(async (raw: string) => {
    if (raw.trim() === "/team") setCooperativeView(false);
    await runCliCommand(raw, {
      manageAgents,
      newSession: () => {
        setCurrentSession("");
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
      checkpoint: handleCheckpointCommand,
      compactContext: handleCompactContext,
      showContext: handleShowContext,
      usage: handleUsageCommand,
      background: async (input) => {
        const socket = socketRef.current;
        const parts = input.split(/\s+/).filter(Boolean);
        const sub = parts[0]?.toLowerCase() || "";
        const targetId = parts[1] || "";
        if (!sub) {
          await loadBackgroundTasksMenu();
          return;
        }
        if (sub === "stop" && !targetId) {
          appendSystem(socket?.cancelBackground(sessionRef.current.id) ? "Background cancellation requested." : "WebSocket is not connected.");
          return;
        }
        if ((sub === "stop" || sub === "retry" || sub === "continue") && targetId) {
          try {
            await apiRef.current.actOnBackgroundTask(targetId, sub);
            appendSystem(`Background task ${targetId}: ${sub} requested.`);
          } catch (error) {
            appendSystem(`Background action failed: ${(error as Error).message}`);
          }
          return;
        }
        appendSystem("Usage: /background [stop [taskId] | retry <taskId> | continue <taskId>]");
      },
      privacy,
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
    setCurrentSession,
    manageAgents,
    loadBackgroundTasksMenu,
    handleCheckpointCommand,
    handleCompactContext,
    handleShowContext,
    handleUsageCommand,
    loadMCPConfigs,
    loadModels,
    loadProviders,
    loadMemory,
    privacy,
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
  handleCommandRef.current = handleCommand;

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
    const handleResize = () => { setWidth(Math.max(20, stdout?.columns || 80)); setTerminalRows(stdout?.rows || 24); };
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
    invalidateSessionRequests,
    resyncSessionHistory,
    applyTerminalTitle,
    onBackgroundCountChange: setBackgroundTaskCount,
  });

  const workspaceLayout = terminalWorkspaceLayout(width, terminalRows, [
    `SlimeBot CLI ${state.version}${state.updateCheck?.updateAvailable ? " [new]" : ""}${state.approvalMode === "auto_review" ? " [auto review]" : state.approvalMode === "auto" ? " [auto]" : ""}`,
    `${state.modelName || "(none)"}${state.thinkingLevel && state.thinkingLevel !== "off" ? ` [think:${state.thinkingLevel}]` : ""}`,
    state.cwd,
  ], Boolean(state.contextUsage));
  const menuPageSize = state.view === "menu" ? getMenuViewport(
    state.menuItems, state.menuCursor, width, workspaceLayout.height, state.menuHint,
    state.menuKind === "session" ? MENU_VISIBLE_LIMIT : undefined,
  ).items.length : 1;

  useCliKeyboard({
    state,
    cooperativeView,
    menuPageSize,
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
    handleBackgroundAction,
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
  const budgetStatus = getOutputBudgetStatus(state.contextUsage);
  const isFullSubview = state.view === "menu"
    || state.view === "checkpoint"
    || state.view === "usage"
    || state.view === "context"
    || (state.view === "team-detail" && cooperativeView);

  return (
    <Box flexDirection="column">
      {isFullSubview && workspaceLayout.compactBanner ? (
        <Text bold color={CLI_ACCENT_COLOR} wrap="truncate">SlimeBot CLI {state.version} · {state.modelName || "(none)"}</Text>
      ) : <Banner
        version={state.version}
        modelName={state.modelName}
        cwd={state.cwd}
        sessionName={state.sessionName}
        approvalMode={state.approvalMode}
        thinkingLevel={state.thinkingLevel}
        updateAvailable={Boolean(state.updateCheck?.updateAvailable)}
      />}
      <Text> </Text>
      {!isFullSubview && state.view !== "team-detail" && (state.timeline.length > 0 || state.streaming) && (
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
      {(state.contextUsage || budgetStatus || backgroundTaskCount > 0) && (
        <Box justifyContent="space-between">
          <Text>
            {state.contextUsage && (
              <Text color={state.contextUsage.usedPercent >= 90 ? CLI_THEME.error : state.contextUsage.usedPercent >= 70 ? CLI_THEME.warning : CLI_THEME.success}>
                {formatContextUsageStatus(state.contextUsage, width)}
              </Text>
            )}
            {budgetStatus && (budgetStatus.tone === "warning" || budgetStatus.tone === "danger") && (
              <Text color={budgetStatus.tone === "danger" ? CLI_THEME.error : CLI_THEME.warning} bold>
                {state.contextUsage ? " · " : ""}
                {budgetStatus.exceeded ? "BUDGET EXCEEDED (/usage budget)" : `BUDGET ${budgetStatus.percent}%`}
              </Text>
            )}
          </Text>
          {backgroundTaskCount > 0 && (
            <Text color={CLI_THEME.accent} bold>BG {backgroundTaskCount}</Text>
          )}
        </Box>
      )}
      <Text color={CLI_THEME.border}>{border}</Text>

      {state.view === "chat" && (
        <Box>
          <Text bold color={state.streaming ? CLI_THEME.muted : CLI_THEME.accent}>
            ❯{" "}
          </Text>
          <TextInput
            key={state.inputKey}
            value={state.inputValue}
            placeholder={state.streaming ? "Type to steer current turn, or press Esc to cancel..." : "Ask anything or type / for commands..."}
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
              const meta = getCommandMeta(completed);
              const next = meta?.acceptsArgs ? `${completed} ` : completed;
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
                const resolved = resolveCommandHintEnter(state.inputValue, selectedCommandHintIndex);
                if (!resolved) return false;
                if (resolved.action === "run") {
                  dispatch({ type: "SET_INPUT_WITH_KEY", value: "" } as AppAction);
                  void handleCommand(resolved.command);
                  return true;
                }
                dispatch({
                  type: "SET_INPUT_WITH_KEY",
                  value: resolved.command,
                } as AppAction);
                return true;
              }
              return false;
            }}
            onUnhandledInput={(input, key) => {
              if (state.view !== "chat" || state.streaming) return;
              handleChatShortcut(input, key, dispatch);
            }}
            focus={state.view === "chat"}
            columns={Math.max(20, width - 3)}
          />
        </Box>
      )}

      {state.view === "checkpoint" && state.sessionId && (
        <CheckpointView
          sessionId={state.sessionId}
          busy={state.streaming || backgroundTaskCount > 0}
          columns={width}
          height={workspaceLayout.height}
          listCheckpoints={(sid) => apiRef.current.listCheckpoints(sid)}
          getCheckpoint={(sid, cpId) => apiRef.current.getCheckpoint(sid, cpId)}
          diffCheckpoint={(sid, cpId, filePath) => apiRef.current.diffCheckpoint(sid, cpId, filePath)}
          rollbackCheckpoint={(sid, cpId, mode) => apiRef.current.rollbackCheckpoint(sid, cpId, mode)}
          onRolledBack={(message) => appendSystem(message)}
          onClose={() => dispatch({ type: "SET_VIEW", view: "chat" } as AppAction)}
        />
      )}

      {state.view === "usage" && (
        <UsageView
          columns={width}
          height={workspaceLayout.height}
          outputBudgetTokens={outputBudgetTokens || state.contextUsage?.outputBudgetTokens}
          loadSummary={(groupBy, opts) => apiRef.current.getUsageSummary(groupBy, opts)}
          onClose={() => dispatch({ type: "SET_VIEW", view: "chat" } as AppAction)}
        />
      )}

      {state.view === "context" && (
        <ContextView
          usage={state.contextUsage}
          modelName={state.modelName}
          streaming={state.streaming}
          compacting={contextCompacting}
          columns={width}
          onCompact={() => { void handleCompactContext(); }}
          onRefresh={() => {
            if (state.sessionId && state.modelId) {
              void refreshContextUsage(state.sessionId, state.modelId);
            }
          }}
          onClose={() => dispatch({ type: "SET_VIEW", view: "chat" } as AppAction)}
        />
      )}

      {state.view === "menu" && state.menuKind && (
        <MenuView
          title={state.menuTitle}
          items={state.menuItems}
          cursor={state.menuCursor}
          hint={state.menuHint}
          columns={width}
          maxHeight={workspaceLayout.height}
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
          <Text bold color={CLI_THEME.accent}>{"Thinking Detail"}</Text>
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

      {state.view === "team-detail" && cooperativeView && <CooperativeView
        snapshot={cooperativeSnapshot} initialTab={cooperativeTab} columns={width}
        height={workspaceLayout.height}
        loading={cooperativeLoading} error={cooperativeError}
        refresh={() => void refreshAgents()}
        onClose={() => dispatch({type:"SET_VIEW",view:"chat"} as AppAction)}
        onCommand={command => manageAgents(command, true)}
        loadReport={async (id, turnId) => {
          const turns = await apiRef.current.cooperativeTurns(id, turnId);
          const turn = turns.find(t => t.id === turnId);
          if (!turn) throw new Error("Agent report not found");
          return turn.answer || turn.error || "Waiting for the agent report.";
        }}
      />}
      {state.view === "team-detail" && !cooperativeView && (
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
          contextSizeSource={state.modelEditorContextSizeSource}
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

      <Text color={CLI_THEME.border}>{border}</Text>

      {state.view === "chat" && state.streaming && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          Generating response · Type to steer · ^O tools · Esc cancel
        </Text>
      )}

      {hasCommandHints && (
        <Box flexDirection="column">
          <CommandHints input={state.inputValue} selectedIndex={selectedCommandHintIndex} />
          <Text color={CLI_HINT_COLOR} wrap="truncate">
            ↑↓ select · Enter run/fill · Tab fill · Esc clear
          </Text>
        </Box>
      )}

      {state.view === "chat" && !state.streaming && !hasCommandHints && (
        <Box justifyContent="space-between">
          <Text color={CLI_HINT_COLOR} wrap="truncate">
            {getChatFooterHint(state.planMode, state.approvalMode)}{width >= 88 ? " · ^O tools · ^K compact" : ""}
          </Text>
          <Box>
            {state.planMode && <Text color={CLI_THEME.plan} bold>◆ Plan </Text>}
            {state.approvalMode === "auto_review" && <Text color={CLI_THEME.warning} bold>◆ Auto Review </Text>}
            {state.approvalMode === "auto" && <Text color={CLI_THEME.warning} bold>◆ Auto </Text>}
          </Box>
        </Box>
      )}

      {state.view === "approval" && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          ↑↓ switch · Y approve · N reject · A approve all · R reject all
        </Text>
      )}

      {state.view === "plan-confirm" && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          ↑↓ navigate · Enter select · Esc cancel
        </Text>
      )}

      {state.view === "team-detail" && !cooperativeView && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          ←/→ Team · ↑/↓ member · Esc back
        </Text>
      )}

      {state.view === "mcp-editor" && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          Tab switch · Ctrl+S save · Ctrl+E toggle · Esc back
        </Text>
      )}

      {state.view === "mcp-template" && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          ↑↓ navigate · Enter select · Esc cancel
        </Text>
      )}

      {state.view === "model-editor" && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          Tab switch · Ctrl+S save · Esc back
        </Text>
      )}

      {state.view === "provider-editor" && (
        <Text color={CLI_HINT_COLOR} wrap="truncate">
          Tab switch · Ctrl+S save · Esc back{state.modelEditorFocusIndex === 1 ? " · Enter protocol" : ""}
        </Text>
      )}
    </Box>
  );
}
