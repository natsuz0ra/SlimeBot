import { getAuthToken } from '../utils/authStorage'
import { decodeChatWSEvent, describeChatWSDiagnostic } from '../../../shared/chat-protocol/events.js'
import { dispatchChatWSEvent, type ChatEventHandlers } from '../../../shared/chat-protocol/dispatch.js'

export type {
  BackgroundTaskData, ContextLayerUsage, ContextUsageData, MessageEditedData, RuntimeTodoItem, RuntimeTodoStatus,
  SkillLearningData, SubagentChunkData, SubagentDoneData, SubagentStartData, ThinkingEventData, TodoUpdateData,
  ToolApprovalRequiredData, ToolCallResultData, ToolCallReviewData, ToolCallStartData,
} from '../../../shared/chat-protocol/dispatch.js'

export interface ChatSocketHandlers extends ChatEventHandlers {
  onOpen?: () => void
  onClose?: () => void
  onSocketError?: (error: string) => void
  onStatusChange?: (status: ConnectionStatus, error?: string) => void
}

export type ConnectionStatus = 'connected' | 'reconnecting' | 'disconnected'

function receiveChatSocketMessage(raw: unknown, handlers: ChatSocketHandlers | null) {
  const result = decodeChatWSEvent(raw)
  if (!result.ok) {
    if (handlers?.onProtocolError) handlers.onProtocolError(result.diagnostic)
    else handlers?.onSocketError?.(describeChatWSDiagnostic(result.diagnostic))
  } else if (handlers) {
    dispatchChatWSEvent(result.event, handlers)
  }
  return result
}

export function dispatchChatSocketMessage(raw: unknown, handlers: ChatSocketHandlers | null): boolean {
  return receiveChatSocketMessage(raw, handlers).ok
}

export class ChatSocket {
  private ws: WebSocket | null = null
  private handlers: ChatSocketHandlers | null = null
  private reconnectTimer: number | null = null
  private heartbeatTimer: number | null = null
  private heartbeatTimeoutTimer: number | null = null
  private reconnectAttempt = 0
  private manualClose = false

  private readonly HEARTBEAT_INTERVAL_MS = 60_000
  private readonly HEARTBEAT_TIMEOUT_MS = 10_000
  private readonly RECONNECT_BASE_DELAY_MS = 1_000
  private readonly RECONNECT_MAX_DELAY_MS = 15_000

  connect(handlers: ChatSocketHandlers) {
    this.handlers = handlers
    this.manualClose = false
    this.clearReconnectTimer()

    if (this.ws) {
      this.teardownSocket()
    }
    this.openSocket()
  }

  send(content: string, sessionId: string, modelId: string, attachmentIds?: string[], thinkingLevel?: string, planMode?: boolean, subagentModelId?: string, acpAgentId?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'chat', content, sessionId, modelId, attachmentIds: attachmentIds || [], thinkingLevel: thinkingLevel || 'off', planMode: !!planMode, subagentModelId: subagentModelId || '', acpAgentId: acpAgentId || '' }))
    return true
  }

  sendEdit(messageId: string, content: string, sessionId: string, modelId: string, thinkingLevel?: string, planMode?: boolean, subagentModelId?: string, acpAgentId?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'chat_edit', messageId, content, sessionId, modelId, thinkingLevel: thinkingLevel || 'off', planMode: !!planMode, subagentModelId: subagentModelId || '', acpAgentId: acpAgentId || '' }))
    return true
  }

  sendSteering(content: string, sessionId: string, turnId: string, submissionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN || !turnId) return false
    this.ws.send(JSON.stringify({ type: 'steer', content, sessionId, turnId, submissionId }))
    return true
  }

  watchSession(sessionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false
    this.ws.send(JSON.stringify({ type: 'watch_session', sessionId }))
    return true
  }

  cancelBackground(sessionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false
    this.ws.send(JSON.stringify({ type: 'background_cancel', sessionId }))
    return true
  }

  sendStop(sessionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'stop', sessionId }))
    return true
  }

  sendToolApproval(toolCallId: string, approved: boolean, answers?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    const payload: Record<string, unknown> = { type: 'tool_approve', toolCallId, approved }
    if (answers) payload.answers = answers
    this.ws.send(JSON.stringify(payload))
    return true
  }

  sendPlanApprove(planId: string, sessionId: string, modelId: string, displayContent?: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'plan_approve', planId, sessionId, modelId, displayContent: displayContent || '' }))
    return true
  }

  sendPlanReject(planId: string, sessionId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'plan_reject', planId, sessionId }))
    return true
  }

  close() {
    this.manualClose = true
    this.clearReconnectTimer()
    this.clearHeartbeat()
    // Detach callbacks before closing: an old close event must not clear a newly reopened channel.
    this.teardownSocket()
    this.emitStatus('disconnected')
  }

  private openSocket() {
    const token = getAuthToken()
    if (!token && !window.slimebotDesktop) {
      this.emitStatus('disconnected', 'missing auth token')
      this.handlers?.onSocketError?.('missing auth token')
      return
    }

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsBase = import.meta.env.VITE_WS_URL || `${protocol}//${location.host}`
    const url = window.slimebotDesktop
      ? `${wsBase}/ws/chat`
      : `${wsBase}/ws/chat?${new URLSearchParams({ token }).toString()}`
    this.emitStatus('reconnecting')
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this.reconnectAttempt = 0
      this.emitStatus('connected')
      this.handlers?.onOpen?.()
      this.startHeartbeat()
    }

    this.ws.onmessage = (event) => {
      const result = receiveChatSocketMessage(event.data, this.handlers)
      if (result.ok && result.event.type === 'pong') this.clearHeartbeatTimeout()
    }

    this.ws.onerror = () => {
      this.handlers?.onSocketError?.('websocket error')
    }

    this.ws.onclose = () => {
      this.clearHeartbeat()
      this.ws = null
      this.handlers?.onClose?.()
      if (this.manualClose) return

      this.emitStatus('reconnecting', 'socket closed unexpectedly')
      this.scheduleReconnect()
    }
  }

  private startHeartbeat() {
    this.clearHeartbeat()
    this.heartbeatTimer = window.setInterval(() => {
      this.sendHeartbeat()
    }, this.HEARTBEAT_INTERVAL_MS)
  }

  private sendHeartbeat() {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    try {
      this.ws.send(JSON.stringify({ type: 'ping' }))
      this.clearHeartbeatTimeout()
      this.heartbeatTimeoutTimer = window.setTimeout(() => {
        this.handlers?.onSocketError?.('heartbeat timeout')
        this.ws?.close()
      }, this.HEARTBEAT_TIMEOUT_MS)
    } catch {
      this.handlers?.onSocketError?.('heartbeat send failed')
      this.ws.close()
    }
  }

  private scheduleReconnect() {
    if (this.manualClose) return
    this.clearReconnectTimer()
    const delay = Math.min(this.RECONNECT_BASE_DELAY_MS * 2 ** this.reconnectAttempt, this.RECONNECT_MAX_DELAY_MS)
    this.reconnectAttempt += 1
    this.reconnectTimer = window.setTimeout(() => {
      this.emitStatus('reconnecting')
      this.openSocket()
    }, delay)
  }

  private clearReconnectTimer() {
    if (this.reconnectTimer !== null) {
      window.clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }

  private clearHeartbeatTimeout() {
    if (this.heartbeatTimeoutTimer !== null) {
      window.clearTimeout(this.heartbeatTimeoutTimer)
      this.heartbeatTimeoutTimer = null
    }
  }

  private clearHeartbeat() {
    if (this.heartbeatTimer !== null) {
      window.clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
    this.clearHeartbeatTimeout()
  }

  private teardownSocket() {
    if (!this.ws) return
    this.ws.onopen = null
    this.ws.onmessage = null
    this.ws.onclose = null
    this.ws.onerror = null
    this.ws.close()
    this.ws = null
    this.clearHeartbeat()
  }

  private emitStatus(status: ConnectionStatus, error?: string) {
    this.handlers?.onStatusChange?.(status, error)
  }
}
