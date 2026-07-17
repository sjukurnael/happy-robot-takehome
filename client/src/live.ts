import { getIdentity } from './identity'
import type { WsEvent } from './types'

type Listener = (evt: WsEvent) => void
type ConnectionListener = (connected: boolean) => void

const RECONNECT_DELAY_MS = 2000

// Single shared WebSocket for the whole app. Components used to each
// open their own connection via useWsEvents, which meant N components
// mounted = N sockets to the same server for the same client. Now
// there's exactly one per tab, which also gives us a single place to
// carry identity and "viewing" state.
class LiveConnection {
  private ws: WebSocket | null = null
  private listeners = new Set<Listener>()
  private connectionListeners = new Set<ConnectionListener>()
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  connected = false
  readonly me = getIdentity()

  private connect() {
    if (this.ws) return
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const params = new URLSearchParams({ clientId: this.me.clientId, name: this.me.name })
    const ws = new WebSocket(`${protocol}//${location.host}/ws?${params}`)
    ws.onopen = () => {
      this.connected = true
      this.connectionListeners.forEach((l) => l(true))
    }
    ws.onmessage = (msg) => {
      let evt: WsEvent
      try {
        evt = JSON.parse(msg.data)
      } catch {
        return
      }
      this.listeners.forEach((l) => l(evt))
    }
    ws.onclose = () => {
      if (this.ws === ws) this.ws = null
      this.connected = false
      this.connectionListeners.forEach((l) => l(false))
      if (!this.reconnectTimer) {
        this.reconnectTimer = setTimeout(() => {
          this.reconnectTimer = null
          this.connect()
        }, RECONNECT_DELAY_MS)
      }
    }
    this.ws = ws
  }

  subscribe(listener: Listener): () => void {
    this.connect()
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }

  // Calls back immediately with the current state, then on every change.
  subscribeConnection(listener: ConnectionListener): () => void {
    this.connect()
    listener(this.connected)
    this.connectionListeners.add(listener)
    return () => this.connectionListeners.delete(listener)
  }

  setViewing(projectId: string, taskId?: string) {
    this.send({ type: 'viewing', projectId, taskId: taskId ?? '' })
  }

  rename(name: string) {
    this.send({ type: 'rename', name })
  }

  private send(payload: object) {
    this.connect()
    const ws = this.ws!
    const data = JSON.stringify(payload)
    if (ws.readyState === WebSocket.OPEN) {
      ws.send(data)
    } else {
      ws.addEventListener('open', () => ws.send(data), { once: true })
    }
  }
}

export const live = new LiveConnection()
