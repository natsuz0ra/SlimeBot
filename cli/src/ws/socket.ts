/**
 * WebSocket client for Go backend /ws/chat: streaming chat + tool approval.
 * Ported from frontend/src/api/chatSocket.ts; uses the ws package instead of browser WebSocket.
 */

import WebSocket from "ws";
import type { AgentTeamMemberRun, AgentTeamRun, ContextUsage, SubagentChunkData, SubagentDoneData, SubagentStartData, TodoUpdateData, ToolApprovalRequiredData, ToolCallReviewData, ToolCallStartData, ToolCallResultData } from "../types.js";

export interface ThinkingEventData {
  content?: string;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
}

export interface WSHandlers {
  onSession: (sessionId: string) => void;
  onStart: (sessionId?: string) => void;
  onChunk: (chunk: string, sessionId?: string) => void;
  onSessionTitle?: (title: string, sessionId?: string) => void;
  onDone: (
    sessionId?: string,
    meta?: { isInterrupted?: boolean; isStopPlaceholder?: boolean; planId?: string; planBody?: string; narration?: string },
  ) => void;
  onError: (error: string, sessionId?: string) => void;
  onToolCallStart?: (data: ToolCallStartData, sessionId?: string) => void;
  onToolCallReview?: (data: ToolCallReviewData, sessionId?: string) => void;
  onToolApprovalRequired?: (data: ToolApprovalRequiredData, sessionId?: string) => void;
  onToolCallResult?: (data: ToolCallResultData, sessionId?: string) => void;
  onTeamStart?: (data: Omit<AgentTeamRun, "members">, sessionId?: string) => void;
  onTeamMemberQueued?: (data: AgentTeamMemberRun, sessionId?: string) => void;
  onTeamDone?: (data: Omit<AgentTeamRun, "members">, sessionId?: string) => void;
  onSubagentStart?: (data: SubagentStartData, sessionId?: string) => void;
  onSubagentChunk?: (data: SubagentChunkData, sessionId?: string) => void;
  onSubagentDone?: (data: SubagentDoneData, sessionId?: string) => void;
  onThinkingStart?: (data: ThinkingEventData) => void;
  onThinkingChunk?: (data: ThinkingEventData) => void;
  onThinkingDone?: (data: ThinkingEventData) => void;
  onTodoUpdate?: (data: TodoUpdateData, sessionId?: string) => void;
  onContextUsage?: (data: ContextUsage, sessionId?: string) => void;
  onContextCompacted?: (data: ContextUsage, sessionId?: string) => void;
  onPlanBody?: (content: string, sessionId?: string) => void;
  onPlanChunk?: (chunk: string, sessionId?: string) => void;
  onPlanStart?: () => void;
}

interface WSIncoming {
  type: string;
  sessionId?: string;
  content?: string;
  answer?: string;
  title?: string;
  error?: string;
  toolCallId?: string;
  toolName?: string;
  command?: string;
  params?: Record<string, unknown>;
  requiresApproval?: boolean;
  reviewStatus?: string;
  reviewRisk?: string;
  reviewReason?: string;
  preamble?: string;
  status?: string;
  output?: string;
  metadata?: unknown;
  isInterrupted?: boolean;
  isStopPlaceholder?: boolean;
  parentToolCallId?: string;
  subagentRunId?: string;
  teamRunId?: string;
  memberRunId?: string;
  requestId?: string;
  maxMembers?: number;
  maxParallel?: number;
  lastError?: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt?: string;
  task?: string;
  planId?: string;
  planBody?: string;
  narration?: string;
  items?: TodoUpdateData["items"];
  note?: string;
  updatedAt?: string;
  usage?: ContextUsage;
  modelConfigId?: string;
  usedTokens?: number;
  totalTokens?: number;
  usedPercent?: number;
  availablePercent?: number;
  isCompacted?: boolean;
  compactedAt?: string;
  inputBudget?: number;
  outputReserve?: number;
  compactionBeforeTokens?: number;
  compactionAfterTokens?: number;
  compactionReason?: string;
  source?: "estimated" | "provider-reported";
  state?: "compacting" | "ready";
}

function teamIdentity(msg: WSIncoming): { teamRunId?: string; memberRunId?: string } {
  return {
    ...(msg.teamRunId ? { teamRunId: msg.teamRunId } : {}),
    ...(msg.memberRunId ? { memberRunId: msg.memberRunId } : {}),
  };
}

function normalizeTeamRun(msg: WSIncoming): Omit<AgentTeamRun, "members"> {
  return {
    id: msg.teamRunId || "",
    sessionId: msg.sessionId || "",
    requestId: msg.requestId || "",
    status: (msg.status || "running") as AgentTeamRun["status"],
    maxMembers: Number(msg.maxMembers) || 8,
    maxParallel: Number(msg.maxParallel) || 4,
    lastError: msg.lastError,
    startedAt: msg.startedAt || msg.createdAt || "",
    finishedAt: msg.finishedAt,
    createdAt: msg.createdAt || msg.startedAt,
    updatedAt: msg.updatedAt || msg.finishedAt,
  };
}

function normalizeTeamMember(msg: WSIncoming): AgentTeamMemberRun {
  return {
    id: msg.memberRunId || "",
    teamRunId: msg.teamRunId || "",
    toolCallId: msg.toolCallId || "",
    subagentRunId: msg.subagentRunId,
    title: msg.title || "",
    task: msg.task || "",
    modelConfigId: msg.modelConfigId,
    status: (msg.status || "queued") as AgentTeamMemberRun["status"],
    answer: msg.answer,
    error: msg.error,
    startedAt: msg.startedAt,
    finishedAt: msg.finishedAt,
    createdAt: msg.createdAt,
    updatedAt: msg.updatedAt,
  };
}

function normalizeContextUsage(msg: WSIncoming | ContextUsage | undefined, fallbackSessionId?: string): ContextUsage | null {
  if (!msg) return null;
  const source = "usage" in msg && msg.usage ? msg.usage : msg;
  const totalTokens = Number(source.totalTokens);
  const usedTokens = Number(source.usedTokens);
  if (!Number.isFinite(totalTokens) || totalTokens <= 0 || !Number.isFinite(usedTokens)) return null;
  const usedPercent = Number.isFinite(Number(source.usedPercent))
    ? Number(source.usedPercent)
    : Math.round((usedTokens / totalTokens) * 100);
  const availablePercent = Number.isFinite(Number(source.availablePercent))
    ? Number(source.availablePercent)
    : Math.max(0, 100 - usedPercent);
  return {
    sessionId: String(source.sessionId || fallbackSessionId || ""),
    modelConfigId: String(source.modelConfigId || ""),
    usedTokens: Math.max(0, Math.round(usedTokens)),
    totalTokens: Math.max(1, Math.round(totalTokens)),
    usedPercent: Math.max(0, Math.min(100, Math.round(usedPercent))),
    availablePercent: Math.max(0, Math.min(100, Math.round(availablePercent))),
    isCompacted: !!source.isCompacted,
    compactedAt: source.compactedAt,
    ...(Number.isFinite(source.inputBudget) ? { inputBudget: Math.max(0, Number(source.inputBudget)) } : {}),
    ...(Number.isFinite(source.outputReserve) ? { outputReserve: Math.max(0, Number(source.outputReserve)) } : {}),
    ...(Number.isFinite(source.compactionBeforeTokens) ? { compactionBeforeTokens: Math.max(0, Number(source.compactionBeforeTokens)) } : {}),
    ...(Number.isFinite(source.compactionAfterTokens) ? { compactionAfterTokens: Math.max(0, Number(source.compactionAfterTokens)) } : {}),
    ...(source.compactionReason ? { compactionReason: source.compactionReason } : {}),
    ...(source.source ? { source: source.source } : {}),
    ...(source.state ? { state: source.state } : {}),
  };
}

export class CLISocket {
  private ws: WebSocket | null = null;
  private handlers: WSHandlers | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;

  connect(apiURL: string, cliToken: string, handlers: WSHandlers): void {
    this.handlers = handlers;

    // Build WS URL: http → ws, https → wss
    const wsBase = apiURL.replace(/^http/, "ws");
    const url = `${wsBase}/ws/chat`;

    // Pass auth via X-CLI-Token header (matches server middleware)
    this.ws = new WebSocket(url, {
      headers: {
        "X-CLI-Token": cliToken,
      },
    });

    this.ws.on("open", () => {
      this.startHeartbeat();
    });

    this.ws.on("message", (data: WebSocket.Data) => {
      dispatchWSMessage(data.toString(), this.handlers);
    });

    this.ws.on("error", () => {
      this.handlers?.onError("WebSocket connection error");
    });

    this.ws.on("close", () => {
      this.clearHeartbeat();
    });
  }

  send(content: string, sessionId: string, modelId: string, thinkingLevel: string = "off", planMode: boolean = false, subagentModelId: string = ""): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(
      JSON.stringify({
        type: "chat",
        content,
        sessionId,
        modelId,
        attachmentIds: [],
        thinkingLevel,
        planMode,
        subagentModelId,
      }),
    );
    return true;
  }

  sendStop(sessionId: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(JSON.stringify({ type: "stop", sessionId }));
    return true;
  }

  sendToolApproval(toolCallId: string, approved: boolean, answers?: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    const payload: Record<string, unknown> = { type: "tool_approve", toolCallId, approved };
    if (answers) payload.answers = answers;
    this.ws.send(JSON.stringify(payload));
    return true;
  }

  sendPlanApprove(planId: string, sessionId: string, modelId: string, displayContent: string = ""): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(
      JSON.stringify({ type: "plan_approve", planId, sessionId, modelId, displayContent }),
    );
    return true;
  }

  sendPlanReject(planId: string, sessionId: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(
      JSON.stringify({ type: "plan_reject", planId, sessionId }),
    );
    return true;
  }

  sendPlanModify(planId: string, sessionId: string, modelId: string, content: string, thinkingLevel: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(
      JSON.stringify({ type: "plan_modify", planId, sessionId, modelId, content, thinkingLevel }),
    );
    return true;
  }

  close(): void {
    this.clearHeartbeat();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  private startHeartbeat(): void {
    this.clearHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.ws.send(JSON.stringify({ type: "ping" }));
      }
    }, 60_000);
  }

  private clearHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }
}

export function dispatchWSMessage(raw: string, handlers: WSHandlers | null): void {
  let msg: WSIncoming;
  try {
    msg = JSON.parse(raw) as WSIncoming;
  } catch {
    return;
  }

  if (msg.type === "pong") return;

  if (msg.type === "session" && msg.sessionId)
    handlers?.onSession(msg.sessionId);
  if (msg.type === "start") handlers?.onStart(msg.sessionId);
  if (msg.type === "chunk")
    handlers?.onChunk(msg.content || "", msg.sessionId);
  if (msg.type === "session_title") {
    handlers?.onSessionTitle?.(msg.title || "", msg.sessionId);
  }
  if (msg.type === "done") {
    handlers?.onDone(msg.sessionId, {
      isInterrupted: msg.isInterrupted,
      isStopPlaceholder: msg.isStopPlaceholder,
      planId: msg.planId,
      planBody: msg.planBody,
      narration: msg.narration,
    });
  }
  if (msg.type === "error")
    handlers?.onError(msg.error || "unknown error", msg.sessionId);
  if (msg.type === "team_start") handlers?.onTeamStart?.(normalizeTeamRun(msg), msg.sessionId);
  if (msg.type === "team_member_queued") handlers?.onTeamMemberQueued?.(normalizeTeamMember(msg), msg.sessionId);
  if (msg.type === "team_done") handlers?.onTeamDone?.(normalizeTeamRun(msg), msg.sessionId);

  if (msg.type === "tool_call_start") {
    handlers?.onToolCallStart?.(
      {
        toolCallId: msg.toolCallId || "",
        toolName: msg.toolName || "",
        command: msg.command || "",
        params: msg.params || {},
        requiresApproval: !!msg.requiresApproval,
        reviewStatus: msg.reviewStatus || "",
        reviewRisk: msg.reviewRisk || "",
        reviewReason: msg.reviewReason || "",
        preamble: msg.preamble || "",
        parentToolCallId: msg.parentToolCallId,
        subagentRunId: msg.subagentRunId,
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "tool_call_review") {
    handlers?.onToolCallReview?.(
      {
        toolCallId: msg.toolCallId || "",
        toolName: msg.toolName || "",
        command: msg.command || "",
        reviewStatus: msg.reviewStatus || "",
        reviewRisk: msg.reviewRisk || "",
        reviewReason: msg.reviewReason || "",
        parentToolCallId: msg.parentToolCallId,
        subagentRunId: msg.subagentRunId,
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "tool_call_approval_required") {
    handlers?.onToolApprovalRequired?.(
      {
        toolCallId: msg.toolCallId || "",
        toolName: msg.toolName || "",
        command: msg.command || "",
        params: msg.params || {},
        requiresApproval: !!msg.requiresApproval,
        reviewStatus: msg.reviewStatus || "",
        reviewRisk: msg.reviewRisk || "",
        reviewReason: msg.reviewReason || "",
        parentToolCallId: msg.parentToolCallId,
        subagentRunId: msg.subagentRunId,
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "tool_call_result") {
    handlers?.onToolCallResult?.(
      {
        toolCallId: msg.toolCallId || "",
        toolName: msg.toolName || "",
        command: msg.command || "",
        requiresApproval: !!msg.requiresApproval,
        status: (msg.status as ToolCallResultData["status"]) || "completed",
        output: msg.output || "",
        error: msg.error || "",
        metadata: msg.metadata,
        parentToolCallId: msg.parentToolCallId,
        subagentRunId: msg.subagentRunId,
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "subagent_chunk") {
    handlers?.onSubagentChunk?.(
      {
        parentToolCallId: msg.parentToolCallId || "",
        subagentRunId: msg.subagentRunId || "",
        content: msg.content || "",
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "subagent_done") {
    handlers?.onSubagentDone?.(
      {
        parentToolCallId: msg.parentToolCallId || "",
        subagentRunId: msg.subagentRunId || "",
        error: msg.error,
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "thinking_start") {
    handlers?.onThinkingStart?.({
      parentToolCallId: msg.parentToolCallId,
      subagentRunId: msg.subagentRunId,
      ...teamIdentity(msg),
    });
  }
  if (msg.type === "thinking_chunk") {
    handlers?.onThinkingChunk?.({
      content: msg.content || "",
      parentToolCallId: msg.parentToolCallId,
      subagentRunId: msg.subagentRunId,
      ...teamIdentity(msg),
    });
  }
  if (msg.type === "thinking_done") {
    handlers?.onThinkingDone?.({
      parentToolCallId: msg.parentToolCallId,
      subagentRunId: msg.subagentRunId,
      ...teamIdentity(msg),
    });
  }

  if (msg.type === "subagent_start") {
    handlers?.onSubagentStart?.(
      {
        parentToolCallId: msg.parentToolCallId || "",
        subagentRunId: msg.subagentRunId || "",
        title: msg.title || "",
        task: msg.task || "",
        ...teamIdentity(msg),
      },
      msg.sessionId,
    );
  }

  if (msg.type === "todo_update") {
    handlers?.onTodoUpdate?.(
      {
        items: Array.isArray(msg.items) ? msg.items : [],
        note: msg.note,
        updatedAt: msg.updatedAt,
      },
      msg.sessionId,
    );
  }

  if (msg.type === "context_usage") {
    const usage = normalizeContextUsage(msg, msg.sessionId);
    if (usage) handlers?.onContextUsage?.(usage, msg.sessionId);
  }

  if (msg.type === "context_compacted") {
    const usage = normalizeContextUsage(msg.usage, msg.sessionId);
    if (usage) handlers?.onContextCompacted?.(usage, msg.sessionId);
  }

  if (msg.type === "plan_body") {
    handlers?.onPlanBody?.(msg.content || "", msg.sessionId);
  }
  if (msg.type === "plan_chunk") {
    handlers?.onPlanChunk?.(msg.content || "", msg.sessionId);
  }
  if (msg.type === "plan_start") {
    handlers?.onPlanStart?.();
  }
}
