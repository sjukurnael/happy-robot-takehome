import type { BreakdownSuggestion, Task } from './types'

export function isTaskBlocked(task: Task, tasksById: Map<string, Task>): boolean {
  return task.dependencies.some((depId) => tasksById.get(depId)?.status !== 'done')
}

export function countBlocked(tasks: Task[]): number {
  const tasksById = new Map(tasks.map((t) => [t.id, t]))
  return tasks.filter((t) => isTaskBlocked(t, tasksById)).length
}

// remapSelectedSuggestions builds the breakdown apply payload: drops
// deselected suggestions and remaps every dependsOn index so it refers to
// positions in the submitted array. A dependency on a deselected
// suggestion is dropped with it — the server only ever sees a
// self-consistent batch.
export function remapSelectedSuggestions(
  subs: BreakdownSuggestion[],
  selected: boolean[],
): BreakdownSuggestion[] {
  const newIndex = new Map<number, number>()
  subs.forEach((_, i) => {
    if (selected[i]) newIndex.set(i, newIndex.size)
  })
  return subs
    .filter((_, i) => selected[i])
    .map((s) => ({
      ...s,
      dependsOn: s.dependsOn.filter((d) => newIndex.has(d)).map((d) => newIndex.get(d)!),
    }))
}
