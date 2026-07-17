import { live } from './live'
import type { PresenceEntry } from './types'

export function othersViewing(entries: PresenceEntry[]): PresenceEntry[] {
  return entries.filter((e) => e.clientId !== live.me.clientId)
}

export function viewersOfTask(entries: PresenceEntry[], taskId: string): PresenceEntry[] {
  return entries.filter((e) => e.taskId === taskId && e.clientId !== live.me.clientId)
}
