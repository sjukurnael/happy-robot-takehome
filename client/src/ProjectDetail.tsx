import { useEffect, useMemo, useRef, useState } from 'react'
import { api, ApiError } from './api'
import type {
  Project,
  StoredEvent,
  Task,
  TaskCreatedPayload,
  TaskDeletedPayload,
  TaskDependenciesChangedPayload,
  TaskStatus,
  TaskUpdatedPayload,
  WsEvent,
} from './types'
import { useWsEvents, usePresence, useConnectionStatus } from './useWsEvents'
import { othersViewing } from './Presence'
import { Avatar } from './Avatar'
import { KanbanBoard } from './KanbanBoard'
import { NewTaskForm } from './NewTaskForm'
import { TaskPanel } from './TaskPanel'
import { IdentityBadge } from './IdentityBadge'
import { ThemeToggle } from './ThemeToggle'
import { countBlocked } from './taskUtils'
import { live } from './live'

const FLASH_DURATION_MS = 1500

export function ProjectDetail({ projectId, onBack }: { projectId: string; onBack: () => void }) {
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [showNewTask, setShowNewTask] = useState(false)
  const [highlighted, setHighlighted] = useState<Set<string>>(new Set())
  const [banner, setBanner] = useState<string | null>(null)
  const presence = usePresence(projectId)
  const connected = useConnectionStatus()

  // Sync bookkeeping — see handleIncomingEvent/processBuffer/fillGap below.
  // Refs, not state: this is delivery-order bookkeeping the component
  // doesn't render from directly, and it must stay in sync between
  // renders without triggering re-renders on its own.
  const lastSeqRef = useRef(0)
  const subscribedProjectRef = useRef<string | null>(null)
  const pendingRef = useRef<WsEvent[]>([])
  const fillingGapRef = useRef(false)
  const selectedTaskIdRef = useRef<string | null>(null)
  selectedTaskIdRef.current = selectedTaskId

  const refresh = () => {
    api.getProject(projectId).then((p) => {
      setProject(p)
      lastSeqRef.current = p.lastSeq
      // Only the very first "viewing" message for a given project carries
      // the subscribe semantics that trigger server-side replay — send it
      // here, once we actually know the project's current lastSeq, rather
      // than from the taskId-focus effect below (which fires synchronously
      // on every projectId change, before this fetch could possibly have
      // resolved, and would otherwise subscribe with a stale/zero lastSeq
      // and force a full-history replay instead of a precise gap-fill).
      if (subscribedProjectRef.current !== projectId) {
        subscribedProjectRef.current = projectId
        live.setViewing(projectId, selectedTaskIdRef.current ?? undefined, p.lastSeq)
      }
    })
    api.listTasks(projectId).then(setTasks)
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId])

  // Task-focus changes within the same project: presence only. Skipped
  // until refresh() above has completed its initial subscribe for this
  // project (see comment there).
  useEffect(() => {
    if (subscribedProjectRef.current !== projectId) return
    live.setViewing(projectId, selectedTaskId ?? undefined, lastSeqRef.current)
  }, [projectId, selectedTaskId])

  function flash(taskId: string) {
    setHighlighted((prev) => new Set(prev).add(taskId))
    setTimeout(() => {
      setHighlighted((prev) => {
        const next = new Set(prev)
        next.delete(taskId)
        return next
      })
    }, FLASH_DURATION_MS)
  }

  // Applies one durable event's payload directly to local state — no
  // refetch. Idempotent by construction (task.created dedupes by id;
  // merges/filters are no-ops if already applied), which is what makes it
  // safe to re-run for events replayed on subscribe or fetched to fill a
  // gap, even if local state already reflects some of them.
  function applyEvent(evt: Pick<WsEvent, 'eventType' | 'payload'> | StoredEvent) {
    switch (evt.eventType) {
      case 'task.created': {
        const { task } = evt.payload as TaskCreatedPayload
        setTasks((ts) => (ts.some((t) => t.id === task.id) ? ts : [...ts, task]))
        flash(task.id)
        break
      }
      case 'task.updated': {
        const { taskId, changes } = evt.payload as TaskUpdatedPayload
        setTasks((ts) => ts.map((t) => (t.id === taskId ? { ...t, ...changes } : t)))
        flash(taskId)
        break
      }
      case 'task.dependencies_changed': {
        const { taskId, dependsOn } = evt.payload as TaskDependenciesChangedPayload
        setTasks((ts) => ts.map((t) => (t.id === taskId ? { ...t, dependencies: dependsOn } : t)))
        flash(taskId)
        break
      }
      case 'task.deleted': {
        const { taskId, removedFromDependenciesOf } = evt.payload as TaskDeletedPayload
        setTasks((ts) =>
          ts
            .filter((t) => t.id !== taskId)
            .map((t) =>
              removedFromDependenciesOf.includes(t.id)
                ? { ...t, dependencies: t.dependencies.filter((d) => d !== taskId) }
                : t,
            ),
        )
        if (selectedTaskIdRef.current === taskId) setSelectedTaskId(null)
        break
      }
      case 'project.updated': {
        const { changes } = evt.payload as { changes: Partial<Project> }
        setProject((proj) => (proj ? { ...proj, ...changes } : proj))
        break
      }
      // comment.added / comment.deleted: TaskPanel listens for these
      // itself (scoped to whichever task is open) and refetches that
      // task's comments directly — nothing to apply here, but the seq
      // still advances below so gap-detection stays correct.
      default:
        break
    }
  }

  // Applies every prefix of the (seq-sorted) buffer that's now contiguous
  // with lastSeqRef. Whatever's left after that is a genuine gap.
  function processBuffer() {
    let progressed = true
    while (progressed) {
      progressed = false
      pendingRef.current.sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))
      const next = pendingRef.current[0]
      if (next && next.seq === lastSeqRef.current + 1) {
        applyEvent(next)
        lastSeqRef.current = next.seq!
        pendingRef.current.shift()
        progressed = true
      }
    }
    if (pendingRef.current.length > 0) {
      fillGap()
    }
  }

  // Fetches exactly the missing range via the catch-up endpoint, applies
  // it, then re-drains the buffer — which may itself reveal that more
  // arrived (and is still gapped) while the fetch was in flight, in which
  // case this recurses via processBuffer -> fillGap once more.
  //
  // TODO: if lastSeqRef is extremely far behind (e.g. a tab left open for
  // days on a busy project), this could mean repeated 500-event page
  // fetches instead of one cheap full refresh. A "too far behind — just
  // call refresh() instead" fallback would be the fix; not implemented
  // since ordinary gaps here are small. See the matching TODO in
  // server/hub.go's replayTo for the subscribe-time equivalent.
  async function fillGap() {
    if (fillingGapRef.current) return
    fillingGapRef.current = true
    try {
      const missed = await api.listEvents(projectId, lastSeqRef.current)
      for (const ev of missed) {
        if (ev.seq <= lastSeqRef.current) continue
        applyEvent(ev)
        lastSeqRef.current = ev.seq
      }
    } catch {
      // Network hiccup — leave pendingRef as-is; the next live event (or a
      // future gap check) will retry the fetch.
    } finally {
      fillingGapRef.current = false
    }
    processBuffer()
  }

  useWsEvents((evt) => {
    if (evt.projectId !== projectId) return

    if (evt.type === 'project.deleted') {
      onBack()
      return
    }
    if (evt.type !== 'event' || evt.seq === undefined) return // presence.updated etc. — not this hook's concern

    const seq = evt.seq
    if (seq <= lastSeqRef.current) return // duplicate/old — ignore
    pendingRef.current.push(evt)
    processBuffer()
  })

  async function handleMoveTask(taskId: string, status: TaskStatus) {
    const prevTasks = tasks
    setTasks((ts) => ts.map((t) => (t.id === taskId ? { ...t, status } : t)))
    try {
      await api.updateTask(taskId, { status })
    } catch (err) {
      setTasks(prevTasks)
      setBanner(err instanceof ApiError ? err.message : 'Failed to move task')
      setTimeout(() => setBanner(null), 4000)
    }
  }

  const blockedCount = useMemo(() => countBlocked(tasks), [tasks])
  const viewers = othersViewing(presence)

  if (!project) return <p>Loading…</p>

  return (
    <div className="board-page">
      <div className="board-header">
        <div className="board-header-left">
          <button className="breadcrumb-link" onClick={onBack}>
            ← Projects
          </button>
          <div className="tabs-row">
            <span className="tab tab-active">Board</span>
            <span className="tab tab-disabled" title="Not implemented yet">
              Gantt
            </span>
            <span className="tab tab-disabled" title="Not implemented yet">
              List
            </span>
          </div>
        </div>
        <div className="board-header-right">
          <span className={connected ? 'live-pill' : 'reconnecting-pill'}>
            <span className="live-dot" />
            {connected ? 'Live' : 'Reconnecting…'}
          </span>
          {viewers.length > 0 && (
            <div className="avatar-stack">
              {viewers.map((v) => (
                <Avatar key={v.clientId} name={v.name} />
              ))}
            </div>
          )}
          <ThemeToggle />
          <IdentityBadge />
        </div>
      </div>

      <div className="board-title-row">
        <div>
          <h1>{project.name}</h1>
          <p className="board-meta">
            {project.description} · {tasks.length} tasks
            {blockedCount > 0 && ` · ${blockedCount} blocked`}
          </p>
        </div>
        <div className="spacer" />
        <button className="btn-primary" onClick={() => setShowNewTask(true)}>
          + New task
        </button>
      </div>

      {banner && <p className="error-banner">{banner}</p>}

      <KanbanBoard
        tasks={tasks}
        presence={presence}
        highlighted={highlighted}
        onMoveTask={handleMoveTask}
        onOpenTask={setSelectedTaskId}
      />

      {showNewTask && (
        <NewTaskForm projectId={projectId} tasks={tasks} onClose={() => setShowNewTask(false)} onCreated={refresh} />
      )}

      {selectedTaskId && (
        <TaskPanel
          taskId={selectedTaskId}
          tasks={tasks}
          presence={presence}
          onClose={() => setSelectedTaskId(null)}
        />
      )}
    </div>
  )
}
