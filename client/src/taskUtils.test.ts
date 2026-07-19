import { describe, expect, it } from 'vitest'
import type { BreakdownSuggestion, Task } from './types'
import { countBlocked, isTaskBlocked, remapSelectedSuggestions } from './taskUtils'

function makeTask(overrides: Partial<Task> & Pick<Task, 'id'>): Task {
  return {
    projectId: 'p1',
    title: overrides.id,
    status: 'todo',
    assignedTo: [],
    configuration: { priority: 'medium', description: '', tags: [], customFields: {} },
    dependencies: [],
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function byId(tasks: Task[]): Map<string, Task> {
  return new Map(tasks.map((t) => [t.id, t]))
}

describe('isTaskBlocked', () => {
  it('is not blocked with no dependencies', () => {
    const t = makeTask({ id: 'a' })
    expect(isTaskBlocked(t, byId([t]))).toBe(false)
  })

  it('is blocked while any dependency is not done', () => {
    const dep = makeTask({ id: 'dep', status: 'in_progress' })
    const t = makeTask({ id: 'a', dependencies: ['dep'] })
    expect(isTaskBlocked(t, byId([t, dep]))).toBe(true)
  })

  it('is not blocked once every dependency is done', () => {
    const dep1 = makeTask({ id: 'd1', status: 'done' })
    const dep2 = makeTask({ id: 'd2', status: 'done' })
    const t = makeTask({ id: 'a', dependencies: ['d1', 'd2'] })
    expect(isTaskBlocked(t, byId([t, dep1, dep2]))).toBe(false)
  })

  it('is blocked when one of several dependencies is incomplete', () => {
    const done = makeTask({ id: 'd1', status: 'done' })
    const todo = makeTask({ id: 'd2', status: 'todo' })
    const t = makeTask({ id: 'a', dependencies: ['d1', 'd2'] })
    expect(isTaskBlocked(t, byId([t, done, todo]))).toBe(true)
  })

  it('treats a dependency on a missing task as blocking', () => {
    // A dangling dependency id (e.g. mid-sync, before the deletion event's
    // removedFromDependenciesOf patch lands) must fail safe: blocked.
    const t = makeTask({ id: 'a', dependencies: ['ghost'] })
    expect(isTaskBlocked(t, byId([t]))).toBe(true)
  })
})

describe('countBlocked', () => {
  it('returns 0 for an empty list', () => {
    expect(countBlocked([])).toBe(0)
  })

  it('counts only blocked tasks across a dependency chain', () => {
    // build -> test -> deploy, with build done and test in progress:
    // deploy is blocked (test isn't done), test is not (build is done).
    const build = makeTask({ id: 'build', status: 'done' })
    const test = makeTask({ id: 'test', status: 'in_progress', dependencies: ['build'] })
    const deploy = makeTask({ id: 'deploy', dependencies: ['build', 'test'] })
    expect(countBlocked([build, test, deploy])).toBe(1)
  })
})

describe('remapSelectedSuggestions', () => {
  function makeSuggestion(title: string, dependsOn: number[] = []): BreakdownSuggestion {
    return { title, description: '', priority: 'medium', tags: [], dependsOn }
  }

  it('is the identity when everything is selected', () => {
    const subs = [makeSuggestion('a'), makeSuggestion('b', [0]), makeSuggestion('c', [0, 1])]
    expect(remapSelectedSuggestions(subs, [true, true, true])).toEqual(subs)
  })

  it('shifts indices when an earlier suggestion is deselected', () => {
    // Deselecting b: c's dep on a (index 0) stays 0, its dep on b is dropped.
    const subs = [makeSuggestion('a'), makeSuggestion('b'), makeSuggestion('c', [0, 1])]
    const out = remapSelectedSuggestions(subs, [true, false, true])
    expect(out.map((s) => s.title)).toEqual(['a', 'c'])
    expect(out[1].dependsOn).toEqual([0])
  })

  it('drops dependencies on deselected suggestions', () => {
    const subs = [makeSuggestion('a'), makeSuggestion('b', [0])]
    const out = remapSelectedSuggestions(subs, [false, true])
    expect(out).toEqual([makeSuggestion('b')])
  })

  it('remaps a chain that survives around a deselected middle', () => {
    const subs = [
      makeSuggestion('a'),
      makeSuggestion('b', [0]),
      makeSuggestion('c', [0]),
      makeSuggestion('d', [1, 2]),
    ]
    // Drop c: d keeps its dep on b, which is still at index 1.
    const out = remapSelectedSuggestions(subs, [true, true, false, true])
    expect(out.map((s) => s.title)).toEqual(['a', 'b', 'd'])
    expect(out[2].dependsOn).toEqual([1])
  })

  it('returns an empty batch when nothing is selected', () => {
    expect(remapSelectedSuggestions([makeSuggestion('a')], [false])).toEqual([])
  })
})
