import { useEffect, useRef, useState } from 'react'
import { live } from './live'
import type { PresenceEntry, WsEvent } from './types'

export function useWsEvents(onEvent: (evt: WsEvent) => void) {
  const handlerRef = useRef(onEvent)
  handlerRef.current = onEvent

  useEffect(() => live.subscribe((evt) => handlerRef.current(evt)), [])
}

// Tracks the presence roster for a project. Subscribing to the socket
// alone isn't enough for the *initial* roster — the server only pushes
// on change — but since the caller is expected to call live.setViewing
// for this project right away, that itself triggers a broadcast that
// includes us, so the roster arrives within one round trip.
export function usePresence(projectId: string | null): PresenceEntry[] {
  const [roster, setRoster] = useState<PresenceEntry[]>([])

  useEffect(() => {
    setRoster([])
  }, [projectId])

  useWsEvents((evt) => {
    if (evt.type === 'presence.updated' && evt.projectId === projectId) {
      setRoster(evt.presence ?? [])
    }
  })

  return roster
}

export function useConnectionStatus(): boolean {
  const [connected, setConnected] = useState(live.connected)
  useEffect(() => live.subscribeConnection(setConnected), [])
  return connected
}

// Accumulates the latest known presence roster per project from every
// presence.updated event seen, regardless of which project this client is
// currently viewing — the hub broadcasts these to all connected clients, so
// this works without a dedicated "list viewers of project X" endpoint.
export function useAllProjectsPresence(): Record<string, PresenceEntry[]> {
  const [byProject, setByProject] = useState<Record<string, PresenceEntry[]>>({})

  useWsEvents((evt) => {
    if (evt.type === 'presence.updated' && evt.projectId) {
      setByProject((prev) => ({ ...prev, [evt.projectId!]: evt.presence ?? [] }))
    }
  })

  return byProject
}
