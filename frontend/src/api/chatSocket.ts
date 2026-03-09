type Handlers = {
  onSession: (sessionId: string) => void
  onStart: (sessionId?: string) => void
  onChunk: (chunk: string, sessionId?: string) => void
  onSessionTitle: (title: string, sessionId?: string) => void
  onDone: (sessionId?: string) => void
  onError: (error: string, sessionId?: string) => void
  onOpen?: () => void
  onClose?: () => void
  onSocketError?: (error: string) => void
  onStatusChange?: (status: ConnectionStatus, error?: string) => void
}

export type ConnectionStatus = 'connected' | 'reconnecting' | 'disconnected'

type WSIncoming = {
  type: string
  sessionId?: string
  content?: string
  title?: string
  error?: string
}

export class ChatSocket {
  private ws: WebSocket | null = null
  private handlers: Handlers | null = null
  private reconnectTimer: number | null = null
  private heartbeatTimer: number | null = null
  private heartbeatTimeoutTimer: number | null = null
  private reconnectAttempt = 0
  private manualClose = false

  private readonly HEARTBEAT_INTERVAL_MS = 60_000
  private readonly HEARTBEAT_TIMEOUT_MS = 10_000
  private readonly RECONNECT_BASE_DELAY_MS = 1_000
  private readonly RECONNECT_MAX_DELAY_MS = 15_000

  connect(handlers: Handlers) {
    this.handlers = handlers
    this.manualClose = false
    this.clearReconnectTimer()

    if (this.ws) {
      this.teardownSocket()
    }
    this.emitStatus('reconnecting')
    this.openSocket()
  }

  send(content: string, sessionId: string, modelId: string) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.handlers?.onSocketError?.('socket is not connected')
      return false
    }
    this.ws.send(JSON.stringify({ type: 'chat', content, sessionId, modelId }))
    return true
  }

  close() {
    this.manualClose = true
    this.clearReconnectTimer()
    this.clearHeartbeat()
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.emitStatus('disconnected')
  }

  private openSocket() {
    const wsBase = import.meta.env.VITE_WS_URL || 'ws://localhost:8080'
    const url = `${wsBase}/ws/chat`
    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      this.reconnectAttempt = 0
      this.emitStatus('connected')
      this.handlers?.onOpen?.()
      this.startHeartbeat()
    }

    this.ws.onmessage = (event) => {
      let data: WSIncoming
      try {
        data = JSON.parse(event.data) as WSIncoming
      } catch {
        this.handlers?.onSocketError?.('invalid websocket payload')
        return
      }

      if (data.type === 'pong') {
        this.clearHeartbeatTimeout()
        return
      }

      if (data.type === 'session' && data.sessionId) this.handlers?.onSession(data.sessionId)
      if (data.type === 'start') this.handlers?.onStart(data.sessionId)
      if (data.type === 'chunk') this.handlers?.onChunk(data.content || '', data.sessionId)
      if (data.type === 'session_title') this.handlers?.onSessionTitle(data.title || '', data.sessionId)
      if (data.type === 'done') this.handlers?.onDone(data.sessionId)
      if (data.type === 'error') this.handlers?.onError(data.error || 'unknown error', data.sessionId)
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
