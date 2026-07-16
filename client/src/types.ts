export interface Project {
  id: string
  name: string
  description: string
  metadata: Record<string, unknown>
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

export interface PresenceEntry {
  clientId: string
  name: string
  taskId?: string
}

export interface WsEvent {
  type: string
  projectId?: string
  resourceId?: string
  presence?: PresenceEntry[]
}
