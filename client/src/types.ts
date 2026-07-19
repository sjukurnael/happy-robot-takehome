// The structural half of the API contract is GENERATED from the Go
// structs into ./generated/api.ts (run `make gen-types` after changing
// them) — so field names and shapes cannot drift from the backend. This
// file re-exports the generated types, adding only the refinements Go's
// type system can't express: closed string unions and per-payload field
// precision. Client code imports from here, never from ./generated.
import type * as gen from './generated/api'

export type {
  BreakdownResponse,
  BreakdownSuggestion,
  Comment,
  CommentDeletedPayload,
  PresenceEntry,
  Project,
  ProjectStats,
  StoredEvent,
  TaskConfiguration,
  TaskDeletedPayload,
  TaskDependenciesChangedPayload,
} from './generated/api'

// On the wire (and in Go) status is a plain string with three known
// values; the closed union lives here so switches stay exhaustive and
// typos fail to compile.
export type TaskStatus = 'todo' | 'in_progress' | 'done'

export interface Task extends Omit<gen.Task, 'status'> {
  status: TaskStatus
}

// Payload refinements: the generated versions are correct but looser —
// pointer fields come out optional, and Changes maps come out as index
// signatures. These narrow them to what the server actually sends.
export interface TaskCreatedPayload {
  task: Task
}
export interface TaskUpdatedPayload {
  taskId: string
  changes: Partial<Pick<Task, 'title' | 'status' | 'assignedTo' | 'configuration'>>
}
export interface CommentAddedPayload {
  comment: gen.Comment
}
export interface ProjectUpdatedPayload {
  changes: Partial<Pick<gen.Project, 'name' | 'description' | 'metadata'>>
}

// Refinement: Go's []*Task generates as (Task | undefined)[]; the server
// never returns nil entries.
export interface BreakdownApplyResponse {
  created: Task[]
}

// The WS wire envelope (see models.go Event) — durable domain events use
// type "event" with seq/eventType/payload populated; presence.updated and
// the thin project.created/deleted notifications use their own shapes.
export type WsEvent = gen.Event
