/**
 * App — Ink CLI 根组件。
 * 统一管理状态、WebSocket 事件、菜单行为和键盘输入分发。
 */

import React, { useCallback, useEffect, useReducer, useRef } from "react";
import { Box, Text, useApp, useInput, useStdout } from "ink";
import type { Key } from "ink";
import { APIClient } from "./api/client.js";
import { ApprovalView } from "./components/ApprovalView.js";
import { Banner } from "./components/Banner.js";
import { CommandHints } from "./components/CommandHints.js";
import { MCPEditor } from "./components/MCPEditor.js";
import { MenuView } from "./components/MenuView.js";
import { TextInput } from "./components/TextInput.js";
import { Timeline } from "./components/Timeline.js";
import { reducer, createInitialState } from "./reducer.js";
import { completeCommand, isCommand } from "./utils/commands.js";
import { formatTimestamp } from "./utils/format.js";
import { clearScreen, setTerminalTitle } from "./utils/terminal.js";
import { CLISocket } from "./ws/socket.js";
import type {
  AppAction,
  AppState,
  LLMConfig,
  MCPConfig,
  MenuItem,
  Message,
  Session,
  Skill,
  TimelineEntry,
  ToolCallHistoryItem,
  ToolCallResultData,
  ToolCallStartData,
  ToolCallStatus,
} from "./types.js";

/** Detects ctrl+letter keypresses, with a fallback for terminals/OS
 *  combos where Ink's `key.ctrl` flag is not set (e.g. Windows).
 *  Ctrl+A–Z produce raw char codes 1–26. */
function isCtrlKey(input: string, key: Key, letter: string): boolean {
  if (key.ctrl && input === letter) return true;
  const expected = letter.charCodeAt(0) - 96; // 'a'→1, 'b'→2, … 'z'→26
  return input.charCodeAt(0) === expected;
}

interface AppProps {
  apiURL: string;
  cliToken: string;
  version: string;
}

export function mapHistoryMessages(
  messages: Message[],
  toolCallsByMsgId: Record<string, ToolCallHistoryItem[]>,
): TimelineEntry[] {
  const ordered = [...messages].sort((a, b) => (a.seq || 0) - (b.seq || 0));
  const entries: TimelineEntry[] = [];

  for (const msg of ordered) {
    if (msg.isStopPlaceholder) continue;

    if (msg.role === "user") {
      entries.push({ kind: "user", content: msg.content });
      continue;
    }
    if (msg.role === "assistant") {
      // Tool calls happen BEFORE the final assistant response,
      // so insert them before the assistant content.
      const toolCalls = [...(toolCallsByMsgId[msg.id] || [])].sort((a, b) => {
        return new Date(a.startedAt).getTime() - new Date(b.startedAt).getTime();
      });
      for (const tc of toolCalls) {
        entries.push({
          kind: "tool",
          content: tc.output || tc.error || "",
          toolCallId: tc.toolCallId,
          toolName: tc.toolName,
          command: tc.command,
          params: tc.params,
          status: (tc.status || "completed") as ToolCallStatus,
          output: tc.output,
          error: tc.error,
        });
      }
      entries.push({ kind: "assistant", content: msg.content });
      continue;
    }

    entries.push({ kind: "system", content: msg.content });
  }

  return entries;
}

export function App({ apiURL, cliToken, version }: AppProps): React.ReactElement {
  const { exit } = useApp();
  const { stdout } = useStdout();
  const width = Math.max(20, stdout?.columns || 80);
  const border = "─".repeat(width);

  const [state, dispatch] = useReducer(
    reducer,
    createInitialState(apiURL, cliToken, process.cwd(), version),
  );

  const apiRef = useRef(new APIClient(apiURL, cliToken));
  const socketRef = useRef<CLISocket | null>(null);
  const blinkRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const sessionRef = useRef({ id: "", name: "" });
  const clearScreenDeferred = useCallback(() => {
    setImmediate(() => clearScreen());
  }, []);

  const appendSystem = useCallback((content: string) => {
    dispatch({ type: "APPEND_ENTRY", entry: { kind: "system", content } } as AppAction);
  }, []);

  const applyTerminalTitle = useCallback((sessionName = "") => {
    setTerminalTitle(sessionName.trim() || "SlimeBot CLI");
  }, []);

  useEffect(() => {
    sessionRef.current = { id: state.sessionId, name: state.sessionName };
  }, [state.sessionId, state.sessionName]);

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
      if (!settings.defaultModel) return;
      const configs = await apiRef.current.listLLMConfigs();
      const model = configs.find((c) => c.id === settings.defaultModel);
      dispatch({
        type: "SET_MODEL",
        modelId: settings.defaultModel,
        modelName: model?.name || settings.defaultModel,
      } as AppAction);
    } catch {
      // Ignore when no model is configured.
    }
  }, []);

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
        hint: "Arrow keys to navigate, Enter to switch, d to delete, Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load sessions: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadModels = useCallback(async () => {
    try {
      const configs = await apiRef.current.listLLMConfigs();
      const items: MenuItem[] = configs.map((c) => ({
        title: c.name,
        desc: c.model,
        data: c,
      }));
      dispatch({
        type: "SET_MENU",
        kind: "model",
        title: "Model Menu",
        items,
        hint: "Arrow keys to navigate, Enter to set default model, Esc to close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load models: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const loadSkills = useCallback(async () => {
    try {
      const skills = await apiRef.current.listSkills();
      const items: MenuItem[] = skills.map((s: Skill) => ({
        title: s.name,
        desc: s.description,
        data: s,
      }));
      dispatch({
        type: "SET_MENU",
        kind: "skills",
        title: "Skills Menu",
        items,
        hint: "Arrow keys to navigate, d to delete, Esc to close",
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
        hint: "Arrow keys, Enter/e edit, a add, Space toggle, d delete, Esc close",
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load MCP configs: ${(error as Error).message}`);
    }
  }, [appendSystem]);

  const showHelp = useCallback(() => {
    const items: MenuItem[] = [
      { title: "/new", desc: "Create a new chat (lazy session creation)", data: null },
      { title: "/session", desc: "Browse, switch, or delete sessions", data: null },
      { title: "/model", desc: "Switch default model", data: null },
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
    dispatch({ type: "SET_SESSION", sessionId: session.id, sessionName: session.name } as AppAction);
    dispatch({ type: "CLOSE_MENU" } as AppAction);
    applyTerminalTitle(session.name);
    clearScreenDeferred();
    try {
      const history = await apiRef.current.getSessionMessages(session.id);
      dispatch({
        type: "LOAD_HISTORY",
        entries: mapHistoryMessages(
          history.messages,
          history.toolCallsByAssistantMessageId || {},
        ),
      } as AppAction);
    } catch (error) {
      appendSystem(`Failed to load session history: ${(error as Error).message}`);
    }
  }, [appendSystem, applyTerminalTitle, clearScreenDeferred]);

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
        dispatch({ type: "SET_MODEL", modelId: model.id, modelName: model.name } as AppAction);
        dispatch({ type: "CLOSE_MENU" } as AppAction);
        appendSystem(`Model switched to ${model.name}.`);
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
  }, [appendSystem, state.menuKind, switchSession]);

  const handleMenuDelete = useCallback(async (item: MenuItem | undefined) => {
    if (!item || !state.menuKind) return;

    try {
      if (state.menuKind === "session") {
        const session = item.data as Session;
        await apiRef.current.deleteSession(session.id);
        if (session.id === state.sessionId) {
          dispatch({ type: "RESET_SESSION" } as AppAction);
          applyTerminalTitle("");
          clearScreenDeferred();
        }
        await loadSessions();
        return;
      }
      if (state.menuKind === "skills") {
        const skill = item.data as Skill;
        await apiRef.current.deleteSkill(skill.id);
        await loadSkills();
        return;
      }
      if (state.menuKind === "mcp") {
        const mcp = item.data as MCPConfig;
        await apiRef.current.deleteMCPConfig(mcp.id);
        await loadMCPConfigs();
      }
    } catch (error) {
      appendSystem(`Delete failed: ${(error as Error).message}`);
    }
  }, [appendSystem, applyTerminalTitle, clearScreenDeferred, loadMCPConfigs, loadSessions, loadSkills, state.menuKind, state.sessionId]);

  const handleMenuAdd = useCallback(() => {
    if (state.menuKind !== "mcp") return;
    dispatch({
      type: "SET_MCP_EDITOR",
      id: "",
      name: "",
      config: "",
      enabled: true,
    } as AppAction);
  }, [state.menuKind]);

  const handleMenuEdit = useCallback((item: MenuItem | undefined) => {
    if (state.menuKind !== "mcp" || !item) return;
    const mcp = item.data as MCPConfig;
    dispatch({
      type: "SET_MCP_EDITOR",
      id: mcp.id,
      name: mcp.name,
      config: mcp.config,
      enabled: mcp.isEnabled,
    } as AppAction);
  }, [state.menuKind]);

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
      return socketRef.current?.send(content, sid, state.modelId) || false;
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
  }, [appendSystem, applyTerminalTitle, state.modelId, state.sessionId]);

  const handleCommand = useCallback(async (raw: string) => {
    const cmd = raw.trim();
    if (cmd === "/new") {
      dispatch({ type: "RESET_SESSION" } as AppAction);
      applyTerminalTitle("");
      clearScreenDeferred();
      return;
    }
    if (cmd === "/session") {
      await loadSessions();
      return;
    }
    if (cmd === "/model") {
      await loadModels();
      return;
    }
    if (cmd === "/skills") {
      await loadSkills();
      return;
    }
    if (cmd === "/mcp") {
      await loadMCPConfigs();
      return;
    }
    if (cmd === "/help") {
      showHelp();
      return;
    }
    appendSystem(`Unknown command: ${cmd}`);
  }, [appendSystem, applyTerminalTitle, clearScreenDeferred, loadMCPConfigs, loadModels, loadSessions, loadSkills, showHelp]);

  // Initial clear + default model.
  useEffect(() => {
    clearScreen();
    applyTerminalTitle("");
    void loadDefaultModel();
  }, [applyTerminalTitle, loadDefaultModel]);

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
    }, 500);

    return () => {
      if (blinkRef.current) {
        clearInterval(blinkRef.current);
        blinkRef.current = null;
      }
    };
  }, [state.streaming, state.assistantWaiting]);

  // WebSocket lifecycle.
  useEffect(() => {
    const socket = new CLISocket();
    socketRef.current = socket;

    const wsBase = apiURL.replace(/^http/, "ws");
    socket.connect(wsBase, cliToken, {
      onSession: (sessionId) => {
        dispatch({ type: "SET_SESSION", sessionId } as AppAction);
        void refreshSessionName(sessionId);
      },
      onStart: () => {
        dispatch({ type: "STREAM_START" } as AppAction);
      },
      onChunk: (chunk) => {
        dispatch({ type: "STREAM_CHUNK", chunk } as AppAction);
      },
      onDone: (_sid, meta) => {
        dispatch({
          type: "STREAM_DONE",
          error: meta?.isStopPlaceholder ? "Generation stopped." : null,
        } as AppAction);
        const current = sessionRef.current;
        applyTerminalTitle(current.name);
        if (current.id) {
          void refreshSessionName(current.id);
        }
      },
      onError: (error) => {
        dispatch({ type: "STREAM_DONE", error } as AppAction);
      },
      onToolCallStart: (data: ToolCallStartData) => {
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: data.toolCallId,
            toolName: data.toolName,
            command: data.command,
            params: data.params,
            status: data.requiresApproval ? "pending" : "executing",
            content: "",
          },
        } as AppAction);

        if (data.requiresApproval) {
          dispatch({
            type: "SET_APPROVAL",
            toolCallId: data.toolCallId,
            toolName: data.toolName,
            command: data.command,
            params: data.params,
            replyCh: () => {},
          } as AppAction);
        }
      },
      onToolCallResult: (data: ToolCallResultData) => {
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: data.toolCallId,
            toolName: data.toolName,
            command: data.command,
            status: data.status || "completed",
            output: data.output,
            error: data.error,
            content: data.output || data.error || "",
          },
        } as AppAction);
      },
    });

    return () => {
      socket.close();
    };
  }, [apiURL, cliToken]);

  useInput((input, key) => {
    if (key.ctrl && input === "c") {
      socketRef.current?.close();
      exit();
      return;
    }

    if (state.view === "approval") {
      if (input === "y" || input === "Y") {
        state.approvalReplyCh?.(true);
        socketRef.current?.sendToolApproval(state.approvalToolCallId, true);
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: state.approvalToolCallId,
            status: "executing",
            content: "",
          },
        } as AppAction);
        dispatch({ type: "CLEAR_APPROVAL" } as AppAction);
      } else if (input === "n" || input === "N" || key.escape) {
        state.approvalReplyCh?.(false);
        socketRef.current?.sendToolApproval(state.approvalToolCallId, false);
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: state.approvalToolCallId,
            status: "rejected",
            error: "Execution was rejected by the user.",
            content: "",
          },
        } as AppAction);
        dispatch({ type: "CLEAR_APPROVAL" } as AppAction);
      }
      return;
    }

    if (state.view === "menu") {
      const current = state.menuItems[state.menuCursor];
      if (key.upArrow) {
        dispatch({ type: "MENU_NAV", delta: -1 } as AppAction);
        return;
      }
      if (key.downArrow) {
        dispatch({ type: "MENU_NAV", delta: 1 } as AppAction);
        return;
      }
      if (key.return) {
        void handleMenuSelect(current);
        return;
      }
      if (key.escape) {
        dispatch({ type: "CLOSE_MENU" } as AppAction);
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
        dispatch({ type: "TOGGLE_MCP_EDITOR_FOCUS" } as AppAction);
        return;
      }
      if (key.escape) {
        void loadMCPConfigs();
        return;
      }
      if (key.ctrl && input === "e") {
        dispatch({ type: "TOGGLE_MCP_EDITOR_ENABLED" } as AppAction);
        return;
      }
      if (key.ctrl && input === "s") {
        void saveMCPConfig();
      }
      return;
    }

    if (state.view !== "chat") return;

    if (isCtrlKey(input, key, "k")) {
      dispatch({ type: "TOGGLE_COMPACT" } as AppAction);
      return;
    }

    if (isCtrlKey(input, key, "o")) {
      dispatch({ type: "TOGGLE_TOOL_OUTPUT" } as AppAction);
      return;
    }

    if (state.streaming) return;
  });

  return (
    <Box flexDirection="column">
      <Banner version={state.version} modelName={state.modelName} cwd={state.cwd} />
      <Text> </Text>
      {(state.timeline.length > 0 || state.streaming) && (
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
          />
          <Text> </Text>
        </>
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
              if (isCommand(value)) {
                void handleCommand(value);
              } else {
                void sendMessage(value);
              }
            }}
            onTab={() => {
              const completed = completeCommand(state.inputValue);
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
          kind={state.menuKind}
          onSelect={() => {}}
          onBack={() => dispatch({ type: "CLOSE_MENU" } as AppAction)}
        />
      )}

      {state.view === "approval" && (
        <ApprovalView
          toolName={state.approvalToolName}
          command={state.approvalCommand}
          params={state.approvalParams}
        />
      )}

      {state.view === "mcp-editor" && (
        <MCPEditor
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

      <Text color="white">{border}</Text>

      {state.view === "chat" && state.streaming && (
        <Text color="gray" dimColor>
          Generating response... Esc to cancel.
        </Text>
      )}

      {state.view === "chat" && !state.streaming && state.inputValue.startsWith("/") && (
        <Box flexDirection="column">
          <CommandHints input={state.inputValue} />
          <Text color="gray" dimColor>
            Tab to autocomplete, Enter to run, Esc to clear.
          </Text>
        </Box>
      )}

      {state.view === "chat" && !state.streaming && !state.inputValue.startsWith("/") && (
        <Text color="gray" dimColor>
          Enter to send | / for commands | Tab to autocomplete | Ctrl+K compact | Ctrl+O expand output | Esc to cancel
        </Text>
      )}

      {state.view === "approval" && (
        <Text color="gray" dimColor>
          Y to approve | N/Esc to reject
        </Text>
      )}

      {state.view === "mcp-editor" && (
        <Text color="gray" dimColor>
          Tab switch field | Ctrl+S save | Ctrl+E toggle | Esc back
        </Text>
      )}
    </Box>
  );
}
