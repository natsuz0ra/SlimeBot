/**
 * WebSocket 客户端：与 Go 后端 /ws/chat 通信，支持流式聊天 + 工具审批。
 * 从 frontend/src/api/chatSocket.ts 移植，使用 ws 包替代浏览器 WebSocket。
 */

import WebSocket from "ws";
import type { ToolCallStartData, ToolCallResultData } from "../types.js";

export interface WSHandlers {
  onSession: (sessionId: string) => void;
  onStart: (sessionId?: string) => void;
  onChunk: (chunk: string, sessionId?: string) => void;
  onDone: (
    sessionId?: string,
    meta?: { isInterrupted?: boolean; isStopPlaceholder?: boolean },
  ) => void;
  onError: (error: string, sessionId?: string) => void;
  onToolCallStart?: (data: ToolCallStartData, sessionId?: string) => void;
  onToolCallResult?: (data: ToolCallResultData, sessionId?: string) => void;
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
}

export class CLISocket {
  private ws: WebSocket | null = null;
  private handlers: WSHandlers | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;

  connect(apiURL: string, cliToken: string, handlers: WSHandlers): void {
    this.handlers = handlers;

    // 构建 WS URL：http → ws, https → wss
    const wsBase = apiURL.replace(/^http/, "ws");
    const url = `${wsBase}/ws/chat`;

    // 通过 X-CLI-Token header 传递认证信息（与服务端中间件匹配）
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
          },
          msg.sessionId,
        );
      }
    });

    this.ws.on("error", () => {
      this.handlers?.onError("WebSocket connection error");
    });

    this.ws.on("close", () => {
      this.clearHeartbeat();
    });
  }

  send(content: string, sessionId: string, modelId: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(
      JSON.stringify({
        type: "chat",
        content,
        sessionId,
        modelId,
        attachmentIds: [],
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
