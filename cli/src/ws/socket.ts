/**
 * WebSocket client for Go backend /ws/chat: streaming chat + tool approval.
 * Ported from frontend/src/api/chatSocket.ts; uses the ws package instead of browser WebSocket.
 */

import WebSocket from "ws";
import type { SubagentChunkData, ToolCallStartData, ToolCallResultData } from "../types.js";

export interface WSHandlers {
  onSession: (sessionId: string) => void;
  onStart: (sessionId?: string) => void;
  onChunk: (chunk: string, sessionId?: string) => void;
  onDone: (
    sessionId?: string,
    meta?: { isInterrupted?: boolean; isStopPlaceholder?: boolean; planId?: string; planBody?: string; narration?: string },
  ) => void;
  onError: (error: string, sessionId?: string) => void;
  onToolCallStart?: (data: ToolCallStartData, sessionId?: string) => void;
  onToolCallResult?: (data: ToolCallResultData, sessionId?: string) => void;
  onSubagentChunk?: (data: SubagentChunkData, sessionId?: string) => void;
  onThinkingStart?: () => void;
  onThinkingChunk?: (chunk: string) => void;
  onThinkingDone?: () => void;
  onPlanBody?: (content: string, sessionId?: string) => void;
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
  params?: Record<string, string>;
  requiresApproval?: boolean;
  preamble?: string;
  status?: string;
  output?: string;
  isInterrupted?: boolean;
  isStopPlaceholder?: boolean;
  parentToolCallId?: string;
  subagentRunId?: string;
  planId?: string;
  planBody?: string;
  narration?: string;
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
      let msg: WSIncoming;
      try {
        msg = JSON.parse(data.toString()) as WSIncoming;
      } catch {
        return;
      }

      if (msg.type === "pong") return;

      if (msg.type === "session" && msg.sessionId)
        this.handlers?.onSession(msg.sessionId);
      if (msg.type === "start") this.handlers?.onStart(msg.sessionId);
      if (msg.type === "chunk")
        this.handlers?.onChunk(msg.content || "", msg.sessionId);
      if (msg.type === "session_title") {
        // Title update - no action needed for CLI
      }
      if (msg.type === "done") {
        this.handlers?.onDone(msg.sessionId, {
          isInterrupted: msg.isInterrupted,
          isStopPlaceholder: msg.isStopPlaceholder,
          planId: msg.planId,
          planBody: msg.planBody,
          narration: msg.narration,
        });
      }
      if (msg.type === "error")
        this.handlers?.onError(msg.error || "unknown error", msg.sessionId);

      if (msg.type === "tool_call_start") {
        this.handlers?.onToolCallStart?.(
          {
            toolCallId: msg.toolCallId || "",
            toolName: msg.toolName || "",
            command: msg.command || "",
            params: msg.params || {},
            requiresApproval: !!msg.requiresApproval,
            preamble: msg.preamble || "",
            parentToolCallId: msg.parentToolCallId,
            subagentRunId: msg.subagentRunId,
          },
          msg.sessionId,
        );
      }

      if (msg.type === "tool_call_result") {
        this.handlers?.onToolCallResult?.(
          {
            toolCallId: msg.toolCallId || "",
            toolName: msg.toolName || "",
            command: msg.command || "",
            requiresApproval: !!msg.requiresApproval,
            status: (msg.status as ToolCallResultData["status"]) || "completed",
            output: msg.output || "",
            error: msg.error || "",
            parentToolCallId: msg.parentToolCallId,
            subagentRunId: msg.subagentRunId,
          },
          msg.sessionId,
        );
      }

      if (msg.type === "subagent_chunk") {
        this.handlers?.onSubagentChunk?.(
          {
            parentToolCallId: msg.parentToolCallId || "",
            subagentRunId: msg.subagentRunId || "",
            content: msg.content || "",
          },
          msg.sessionId,
        );
      }

      if (msg.type === "thinking_start") {
        this.handlers?.onThinkingStart?.();
      }
      if (msg.type === "thinking_chunk") {
        this.handlers?.onThinkingChunk?.(msg.content || "");
      }
      if (msg.type === "thinking_done") {
        this.handlers?.onThinkingDone?.();
      }

      if (msg.type === "plan_body") {
        this.handlers?.onPlanBody?.(msg.content || "", msg.sessionId);
      }
      if (msg.type === "plan_start") {
        this.handlers?.onPlanStart?.();
      }
    });

    this.ws.on("error", () => {
      this.handlers?.onError("WebSocket connection error");
    });

    this.ws.on("close", () => {
      this.clearHeartbeat();
    });
  }

  send(content: string, sessionId: string, modelId: string, thinkingLevel: string = "off", planMode: boolean = false): boolean {
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
      }),
    );
    return true;
  }

  sendStop(sessionId: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(JSON.stringify({ type: "stop", sessionId }));
    return true;
  }

  sendToolApproval(toolCallId: string, approved: boolean): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(
      JSON.stringify({ type: "tool_approve", toolCallId, approved }),
    );
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
