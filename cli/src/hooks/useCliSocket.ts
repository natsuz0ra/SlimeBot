import { useEffect } from "react";
import type React from "react";
import type { SessionSelection } from "../controllers/sessionRequests.js";
import type { AppAction, ToolCallResultData, ToolCallStartData } from "../types.js";
import { CLISocket } from "../ws/socket.js";
import { describeChatWSDiagnostic } from "../../../shared/chat-protocol/events.js";
import { parseChatTimestamp } from "../../../shared/chat-protocol/timeline.js";

interface UseCliSocketProps {
  apiURL: string;
  cliToken: string;
  socketRef: React.MutableRefObject<CLISocket | null>;
  sessionRef: React.MutableRefObject<SessionSelection>;
  liveAssistantRef: React.MutableRefObject<string>;
  planStartedRef: React.MutableRefObject<boolean>;
  preambleShownRef: React.MutableRefObject<string>;
  dispatch: React.Dispatch<AppAction>;
  refreshSessionName: (sessionId: string) => Promise<void>;
  invalidateSessionRequests: () => void;
  resyncSessionHistory: (sessionId: string) => void;
  applyTerminalTitle: (sessionName?: string) => void;
  onBackgroundCountChange?: (count: number) => void;
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
  invalidateSessionRequests,
  resyncSessionHistory,
  applyTerminalTitle,
  onBackgroundCountChange,
}: UseCliSocketProps): void {
  useEffect(() => {
    const socket = new CLISocket();
    socketRef.current = socket;

    socket.connect(apiURL, cliToken, {
      acceptSession: (sessionId) => sessionId === sessionRef.current.id,
      onOpen: (reconnected) => {
        const { id } = sessionRef.current;
        if (reconnected && id) resyncSessionHistory(id);
      },
      onClose: () => {
        invalidateSessionRequests();
        liveAssistantRef.current = "";
        planStartedRef.current = false;
        preambleShownRef.current = "";
        onBackgroundCountChange?.(0);
        dispatch({ type: "SOCKET_DISCONNECTED" });
      },
      onProtocolError: (diagnostic) => {
        dispatch({ type: "APPEND_ENTRY", entry: { kind: "system", content: describeChatWSDiagnostic(diagnostic) } });
      },
      onBackgroundError: (error) => {dispatch({ type: "APPEND_ENTRY", entry: { kind: "system", content: error } });},
      onBackgroundTasks: (tasks) => {
        socket.backgroundTasks = new Map(tasks.map(task => [task.taskId, task]));
        onBackgroundCountChange?.(socket.backgroundTasks.size);
      },
      onBackgroundTask: (task) => {
        if (["finished", "failed", "stopped", "completed"].includes(task.status)) socket.backgroundTasks.delete(task.taskId);
        else socket.backgroundTasks.set(task.taskId, task);
        onBackgroundCountChange?.(socket.backgroundTasks.size);
        if (task.status === "running") dispatch({ type: "APPEND_ENTRY", entry: { kind: "system", content: `Background task ${task.processId} is running. Completion will resume this conversation. Use /background to inspect or stop.` } });
      },

      onSession: (sessionId) => {
        // The server echoes the session for each submitted turn; a late echo must not switch the view.
        if (sessionId !== sessionRef.current.id) return;
        void refreshSessionName(sessionId);
      },
      onStart: (_sid, meta) => {
        invalidateSessionRequests();
        socket.activeTurnId = meta?.turnId || "";
        planStartedRef.current = false;
        preambleShownRef.current = "";
        dispatch({ type: "STREAM_START" });
      },
      onSteering: (data, sid) => {
        if (sid !== sessionRef.current.id) return;
        if (data.error) {dispatch({ type: "APPEND_ENTRY", entry: { kind: "system", content: data.content ? `${data.error}\nUnsent guidance: ${data.content}` : data.error } }); return;}
        dispatch({ type: "APPEND_ENTRY", entry: { kind: "user", content: data.content || "" } });
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
      onDone: (_sid, _answer, meta) => {
        socket.activeTurnId = "";
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
        if (meta?.skillLearning?.message) {
          dispatch({
            type: "APPEND_ENTRY",
            entry: {
              kind: "system",
              content: meta.skillLearning.skillId
                ? `Skill learning (${meta.skillLearning.status}): ${meta.skillLearning.skillId} — ${meta.skillLearning.message}`
                : `Skill learning (${meta.skillLearning.status}): ${meta.skillLearning.message}`,
            },
          });
        }
        const current = sessionRef.current;
        applyTerminalTitle(current.name);
        if (current.id) {
          void refreshSessionName(current.id);
        }
      },
      onError: (error) => {
        socket.activeTurnId = "";
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
            status: data.reviewStatus === "reviewing" ? "reviewing" : data.requiresApproval ? "pending" : "executing",
            content: "",
            parentToolCallId: data.parentToolCallId,
            subagentRunId: data.subagentRunId,
            teamRunId: data.teamRunId,
            memberRunId: data.memberRunId,
          },
        });

        if (!data.requiresApproval || data.reviewStatus === "reviewing") return;
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
      onToolCallReview: (data) => {
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: data.toolCallId,
            toolName: data.toolName,
            command: data.command,
            status: data.reviewStatus === "approved" ? "executing" : data.reviewStatus === "reviewing" ? "reviewing" : "pending",
            content: data.reviewReason || "",
          },
        });
      },
      onToolApprovalRequired: (data) => {
        dispatch({
          type: "UPSERT_TOOL_ENTRY",
          entry: {
            kind: "tool",
            toolCallId: data.toolCallId,
            toolName: data.toolName,
            command: data.command,
            params: data.params,
            status: "pending",
            content: data.reviewReason || "",
          },
        });
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
            teamRunId: data.teamRunId,
            memberRunId: data.memberRunId,
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
            teamRunId: data.teamRunId,
            memberRunId: data.memberRunId,
          },
        });
        if (data.teamRunId && data.memberRunId) {
          dispatch({
            type: "UPSERT_TEAM_MEMBER",
            member: {
              id: data.memberRunId,
              teamRunId: data.teamRunId,
              toolCallId: data.parentToolCallId,
              subagentRunId: data.subagentRunId,
              title: data.title,
              task: data.task,
              status: "running",
              startedAt: new Date().toISOString(),
            },
          });
        }
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
        if (data.teamRunId && data.memberRunId) {
          dispatch({
            type: "UPSERT_TEAM_MEMBER",
            member: {
              id: data.memberRunId,
              teamRunId: data.teamRunId,
              toolCallId: data.parentToolCallId,
              subagentRunId: data.subagentRunId,
              title: "",
              task: "",
              status: data.error ? "failed" : "succeeded",
              error: data.error,
              finishedAt: new Date().toISOString(),
            },
          });
        }
      },
      onTeamStart: (run) => {
        dispatch({ type: "UPSERT_TEAM_RUN", run });
      },
      onTeamMemberQueued: (member) => {
        dispatch({ type: "UPSERT_TEAM_MEMBER", member });
      },
      onTeamDone: (run) => {
        dispatch({ type: "UPSERT_TEAM_RUN", run });
      },
      onThinkingStart: (data) => {
        dispatch({
          type: "THINKING_START",
          startedAt: parseChatTimestamp(data.startedAt) ?? Date.now(),
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        });
      },
      onThinkingChunk: (data) => {
        dispatch({
          type: "THINKING_CHUNK",
          chunk: data.content || "",
          startedAt: parseChatTimestamp(data.startedAt),
          parentToolCallId: data.parentToolCallId,
          subagentRunId: data.subagentRunId,
        });
      },
      onThinkingDone: (data) => {
        dispatch({
          type: "THINKING_DONE",
          finishedAt: parseChatTimestamp(data.finishedAt) ?? Date.now(),
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
    socket.watchSession(sessionRef.current.id);

    return () => {
      invalidateSessionRequests();
      socketRef.current = null;
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
    invalidateSessionRequests,
    resyncSessionHistory,
    applyTerminalTitle,
    onBackgroundCountChange,
  ]);
}
