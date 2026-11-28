import WebSocket from "ws";
import { decodeChatWSEvent, type ChatWSDecodeResult } from "../../../shared/chat-protocol/events.js";
import { dispatchChatWSEvent, type ChatEventHandlers, type BackgroundTaskData } from "../../../shared/chat-protocol/dispatch.js";

export type { ThinkingEventData, BackgroundTaskData } from "../../../shared/chat-protocol/dispatch.js";

export interface WSHandlers extends ChatEventHandlers {
  onOpen?: (reconnected: boolean) => void;
  onClose?: () => void;
}

export function dispatchWSMessage(raw: unknown, handlers: WSHandlers | null): ChatWSDecodeResult {
  const result = decodeChatWSEvent(raw);
  if (handlers) {
    if (result.ok) dispatchChatWSEvent(result.event, handlers);
    else handlers.onProtocolError?.(result.diagnostic);
  }
  return result;
}

export class CLISocket {
  activeTurnId = "";
  backgroundTasks = new Map<string, BackgroundTaskData>();
  private ws: WebSocket | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private watchedSessionId: string | null = null;

  connect(apiURL: string, cliToken: string, handlers: WSHandlers): void {
    this.close();

    // Build WS URL: http → ws, https → wss
    const wsBase = apiURL.replace(/^http/, "ws");
    const url = `${wsBase}/ws/chat`;

    const initialDelay = 250;
    let reconnectDelay = initialDelay;
    const openSocket = () => {
      const socket = new WebSocket(url, {
        headers: { "X-CLI-Token": cliToken },
        handshakeTimeout: 5_000,
      });
      this.ws = socket;
      socket.on("open", () => {
        if (this.ws !== socket) return;
        const reconnected = reconnectDelay > initialDelay;
        reconnectDelay = initialDelay;
        this.startHeartbeat();
        if (this.watchedSessionId !== null) this.watchSession(this.watchedSessionId);
        handlers.onOpen?.(reconnected);
      });
      socket.on("message", (data: WebSocket.Data) => {
        if (this.ws === socket) dispatchWSMessage(data.toString(), handlers);
      });
      // The transport emits close after an error, including handshake failures.
      socket.on("error", () => {});
      socket.on("close", () => {
        if (this.ws !== socket) return;
        this.ws = null;
        this.activeTurnId = "";
        this.backgroundTasks.clear();
        this.clearHeartbeat();
        const firstDisconnect = reconnectDelay === initialDelay;
        this.reconnectTimer = setTimeout(() => {
          this.reconnectTimer = null;
          openSocket();
        }, reconnectDelay);
        reconnectDelay = Math.min(reconnectDelay * 2, 5_000);
        if (firstDisconnect) handlers.onClose?.();
      });
    };
    openSocket();
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

  sendSteering(content: string, sessionId: string, submissionId: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN || !this.activeTurnId) return false;
    this.ws.send(JSON.stringify({ type: "steer", content, sessionId, turnId: this.activeTurnId, submissionId }));
    return true;
  }

  watchSession(sessionId: string): boolean {
    if (sessionId !== this.watchedSessionId) {
      this.activeTurnId = "";
      this.backgroundTasks.clear();
    }
    this.watchedSessionId = sessionId;
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(JSON.stringify({ type: "watch_session", sessionId }));
    return true;
  }

  cancelBackground(sessionId: string): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(JSON.stringify({ type: "background_cancel", sessionId }));
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
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    const socket = this.ws;
    this.ws = null;
    this.watchedSessionId = null;
    this.activeTurnId = "";
    this.backgroundTasks.clear();
    socket?.close();
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
