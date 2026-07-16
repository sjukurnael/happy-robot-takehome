import { colorForClientId } from './identity'
import { live } from './live'
import type { PresenceEntry } from './types'

export function PresenceRoster({ entries }: { entries: PresenceEntry[] }) {
  const others = entries.filter((e) => e.clientId !== live.me.clientId)
  if (others.length === 0) return null
  return (
    <div className="presence-roster">
      Viewing now:
      {others.map((e) => (
        <span key={e.clientId} className="presence-pill">
          <span className="dot" style={{ background: colorForClientId(e.clientId) }} />
          {e.name}
        </span>
      ))}
    </div>
  )
}

export function viewersOfTask(entries: PresenceEntry[], taskId: string): PresenceEntry[] {
  return entries.filter((e) => e.taskId === taskId && e.clientId !== live.me.clientId)
}
