import type { Task } from './types'

export function isTaskBlocked(task: Task, tasksById: Map<string, Task>): boolean {
  return task.dependencies.some((depId) => tasksById.get(depId)?.status !== 'done')
}

export function countBlocked(tasks: Task[]): number {
  const tasksById = new Map(tasks.map((t) => [t.id, t]))
  return tasks.filter((t) => isTaskBlocked(t, tasksById)).length
}
