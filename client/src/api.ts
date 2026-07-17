import type { Comment, Project, Task, TaskStatus } from './types'

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
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
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
