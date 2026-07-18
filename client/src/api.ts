import type { Comment, Project, StoredEvent, Task, TaskStatus } from './types'
import { getIdentity } from './identity'

// Carries the HTTP status alongside the server's error message so callers
// can distinguish e.g. a 409 (blocked by dependency) from a 400 (malformed
// request) without re-parsing the message.
export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      // Attributes mutations to a display name for the events table's
      // actor column — getIdentity() reads the same sessionStorage-backed
      // identity the WS connection uses (live.ts), so this doesn't prompt
      // again or diverge from it.
      'X-Actor': getIdentity().name,
      ...(init?.headers ?? {}),
    },
  })
  if (!res.ok) {
    const body = await res.text()
    let message = body
    try {
      const parsed = JSON.parse(body)
      if (typeof parsed?.error === 'string') message = parsed.error
    } catch {
      // body wasn't JSON — fall back to raw text
    }
    throw new ApiError(res.status, message)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  listProjects: () => request<Project[]>('/api/projects/'),
  createProject: (data: { name: string; description: string; metadata?: Record<string, unknown> }) =>
    request<Project>('/api/projects/', { method: 'POST', body: JSON.stringify(data) }),
  getProject: (id: string) => request<Project>(`/api/projects/${id}/`),
  updateProject: (id: string, patch: Partial<Pick<Project, 'name' | 'description' | 'metadata'>>) =>
    request<Project>(`/api/projects/${id}/`, { method: 'PATCH', body: JSON.stringify(patch) }),
  deleteProject: (id: string) => request<void>(`/api/projects/${id}/`, { method: 'DELETE' }),

  listTasks: (projectId: string) => request<Task[]>(`/api/projects/${projectId}/tasks`),
  // Catch-up: events for projectId with seq > after, ascending, capped at
  // limit server-side. after=0 returns the whole history — only meant for
  // filling a specific gap, not initial sync (seed lastSeq from the
  // project snapshot's lastSeq instead).
  listEvents: (projectId: string, after: number, limit = 500) =>
    request<StoredEvent[]>(`/api/projects/${projectId}/events?after=${after}&limit=${limit}`),
  createTask: (
    projectId: string,
    data: { title: string; configuration?: Partial<Task['configuration']>; assignedTo?: string[]; dependencies?: string[] },
  ) => request<Task>(`/api/projects/${projectId}/tasks`, { method: 'POST', body: JSON.stringify(data) }),
  getTask: (id: string) => request<Task>(`/api/tasks/${id}/`),
  updateTask: (
    id: string,
    patch: Partial<{
      title: string
      status: TaskStatus
      assignedTo: string[]
      configuration: Task['configuration']
      dependencies: string[]
    }>,
  ) => request<Task>(`/api/tasks/${id}/`, { method: 'PATCH', body: JSON.stringify(patch) }),
  deleteTask: (id: string) => request<void>(`/api/tasks/${id}/`, { method: 'DELETE' }),

  listComments: (taskId: string) => request<Comment[]>(`/api/tasks/${taskId}/comments`),
  createComment: (taskId: string, data: { content: string; author: string }) =>
    request<Comment>(`/api/tasks/${taskId}/comments`, { method: 'POST', body: JSON.stringify(data) }),
  deleteComment: (id: string) => request<void>(`/api/comments/${id}`, { method: 'DELETE' }),
}
