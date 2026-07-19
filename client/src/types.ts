export interface Project {
  id: string
  name: string
  description: string
  metadata: Record<string, unknown>
  lastSeq: number
  createdAt: string
  updatedAt: string
}

export type TaskStatus = 'todo' | 'in_progress' | 'done'

export interface TaskConfiguration {
  priority: string
  description: string
  tags: string[]
  customFields: Record<string, unknown>
}

export interface Task {
  id: string
  projectId: string
  title: string
  status: TaskStatus
  assignedTo: string[]
  configuration: TaskConfiguration
  dependencies: string[]
  createdAt: string
  updatedAt: string
}

export interface Comment {
  id: string
  taskId: string
  content: string
  author: string
  timestamp: string
}

// Per-project dashboard aggregates, computed server-side in one SQL pass
// (GET /api/projects/stats) so the project list never downloads full task
// lists just to render stat cards. "blocked" mirrors taskUtils.ts: a task
// with any not-done dependency.
export interface ProjectStats {
  projectId: string
  total: number
  done: number
  blocked: number
  assignees: string[]
  lastEdited: string
}

export interface PresenceEntry {
  clientId: string
  name: string
  taskId?: string
}

// Payload shapes for each durable event type — see server/events.go.
export interface TaskCreatedPayload {
  task: Task
}
export interface TaskUpdatedPayload {
  taskId: string
  changes: Partial<Pick<Task, 'title' | 'status' | 'assignedTo' | 'configuration'>>
}
export interface TaskDeletedPayload {
  taskId: string
  removedFromDependenciesOf: string[]
}
export interface TaskDependenciesChangedPayload {
  taskId: string
  dependsOn: string[]
}
export interface CommentAddedPayload {
  comment: Comment
}
export interface CommentDeletedPayload {
  commentId: string
  taskId: string
}
export interface ProjectUpdatedPayload {
  changes: Partial<Pick<Project, 'name' | 'description' | 'metadata'>>
}

// A durable event as returned by the catch-up endpoint (GET
// /api/projects/:id/events) — the same shape is embedded in the live WS
// envelope below, so replayed-on-subscribe, fetched-to-fill-a-gap, and
// live-broadcast events can all be applied through one code path.
export interface StoredEvent {
  projectId: string
  seq: number
  eventType: string
  payload: unknown
  actor: string
  createdAt: string
}

// The WS wire envelope. Durable domain events use type "event", with the
// actual event type/seq/payload/actor populated (see StoredEvent above).
// presence.updated and the legacy thin project.deleted notification (both
// intentionally outside the per-project event log) use their own simpler
// shapes via type and resourceId/presence.
export interface WsEvent {
  type: string
  projectId?: string
  resourceId?: string
  seq?: number
  eventType?: string
  payload?: unknown
  actor?: string
  presence?: PresenceEntry[]
}
