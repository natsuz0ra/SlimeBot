import { useEffect } from "react";
import type React from "react";
import type { AppAction, ToolCallResultData, ToolCallStartData } from "../types.js";
import { CLISocket } from "../ws/socket.js";

interface UseCliSocketProps {
  apiURL: string;
  cliToken: string;
  socketRef: React.MutableRefObject<CLISocket | null>;
  sessionRef: React.MutableRefObject<{ id: string; name: string }>;
  liveAssistantRef: React.MutableRefObject<string>;
  planStartedRef: React.MutableRefObject<boolean>;
  preambleShownRef: React.MutableRefObject<string>;
  dispatch: React.Dispatch<AppAction>;
  refreshSessionName: (sessionId: string) => Promise<void>;
  applyTerminalTitle: (sessionName?: string) => void;
}

export function useCliSocket({
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
}: UseCliSocketProps): void {
  useEffect(() => {
    const socket = new CLISocket();
    socketRef.current = socket;

    const wsBase = apiURL.replace(/^http/, "ws");
    socket.connect(wsBase, cliToken, {
      onSession: (sessionId) => {
        dispatch({ type: "SET_SESSION", sessionId });
        void refreshSessionName(sessionId);
      },
      onStart: () => {
        planStartedRef.current = false;
        preambleShownRef.current = "";
        dispatch({ type: "STREAM_START" });
      },
      onChunk: (chunk) => {
        dispatch({ type: "STREAM_CHUNK", chunk });
      },
      onSessionTitle: (title, sessionId) => {
        const trimmed = title.trim();
        if (!trimmed || !sessionId || sessionId !== sessionRef.current.id) return;
        sessionRef.current = { ...sessionRef.current, name: trimmed };
        dispatch({ type: "APPLY_SESSION_TITLE", sessionId, title: trimmed });
        applyTerminalTitle(trimmed);
      },
      onDone: (_sid, meta) => {
        dispatch({
          type: "STREAM_DONE",
          error: meta?.isStopPlaceholder ? "Generation stopped." : null,
        });
        if (meta?.planId) {
          dispatch({
            type: "SET_PLAN_CONFIRMATION",
            planId: meta.planId,
            content: meta.planBody || liveAssistantRef.current || "",
          });
        }
        const current = sessionRef.current;
        applyTerminalTitle(current.name);
        if (current.id) {
          void refreshSessionName(current.id);
        }
      },
      onError: (error) => {
        dispatch({ type: "STREAM_DONE", error });
      },
      onToolCallStart: (data: ToolCallStartData) => {
        const hadLiveText = !!liveAssistantRef.current.trim();
        const preamble = data.preamble?.trim() || "";
        const preambleAlreadyShown = preamble && preamble === preambleShownRef.current;
        dispatch({ type: "FLUSH_AND_WAIT" });
        if (!hadLiveText && preamble && !preambleAlreadyShown) {
          dispatch({
            type: "APPEND_ENTRY",
            entry: { kind: "assistant", content: preamble },
          });
          preambleShownRef.current = preamble;
        }
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
            parentToolCallId: data.parentToolCallId,
            subagentRunId: data.subagentRunId,
          },
        });

        if (!data.requiresApproval) return;
        const questionsRaw = data.params?.questions;
        if (data.toolName === "ask_questions" && typeof questionsRaw === "string") {
          try {
            const questions = JSON.parse(questionsRaw);
            if (Array.isArray(questions) && questions.length > 0) {
              dispatch({
                type: "SET_QA",
                toolCallId: data.toolCallId,
                questions,
              });
            }
          } catch {
            // Ignore malformed ask_questions payloads; backend will resolve the tool.
          }
          return;
        }
        dispatch({
          type: "ADD_PENDING_APPROVAL",
          item: {
            toolCallId: data.toolCallId,
            toolName: data.toolName,
            command: data.command,
            params: data.params,
          },
        });
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
            metadata: data.metadata,
            content: data.output || data.error || "",
            parentToolCallId: data.parentToolCallId,
            subagentRunId: data.subagentRunId,
          },
        });
        dispatch({ type: "REMOVE_PENDING_APPROVAL", toolCallId: data.toolCallId });
      },
      onSubagentStart: (data) => {
        if (!data.parentToolCallId) return;
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: data.parentToolCallId,
            toolName: "run_subagent",
            command: "delegate",
            status: "executing",
            content: "",
            subagentRunId: data.subagentRunId,
            subagentTitle: data.title,
            subagentTask: data.task,
          },
        });
      },
      onSubagentChunk: (data) => {
        if (!data.parentToolCallId || !data.content) return;
        dispatch({
          type: "APPEND_SUBAGENT_STREAM",
          parentToolCallId: data.parentToolCallId,
          content: data.content,
        });
      },
      onSubagentDone: (data) => {
        if (!data.parentToolCallId) return;
        dispatch({
          type: "SUBAGENT_DONE",
          parentToolCallId: data.parentToolCallId,
          error: data.error,
          finishedAt: Date.now(),
        });
      },
      onThinkingStart: (data) => {
        dispatch({
          type: "THINKING_START",
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        });
      },
      onThinkingChunk: (data) => {
        dispatch({
          type: "THINKING_CHUNK",
          chunk: data.content || "",
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        });
      },
      onThinkingDone: (data) => {
        dispatch({
          type: "THINKING_DONE",
          finishedAt: Date.now(),
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        });
      },
      onTodoUpdate: (data) => {
        dispatch({
          type: "TODO_UPDATE",
          items: data.items,
          note: data.note,
          updatedAt: data.updatedAt ? Date.parse(data.updatedAt) : undefined,
        });
      },
      onContextUsage: (usage) => {
        dispatch({ type: "CONTEXT_USAGE", usage });
      },
      onContextCompacted: (usage) => {
        dispatch({ type: "CONTEXT_COMPACTED", usage });
      },
      onPlanBody: (content: string) => {
        dispatch({ type: "PLAN_BODY", planBody: content });
      },
      onPlanChunk: (chunk: string) => {
        dispatch({ type: "PLAN_CHUNK", chunk });
      },
      onPlanStart: () => {
        planStartedRef.current = true;
        dispatch({ type: "PLAN_START" });
      },
    });

    return () => {
      socket.close();
    };
  }, [
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
  ]);
}
