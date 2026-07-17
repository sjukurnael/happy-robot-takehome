import { useEffect, useMemo, useState } from 'react'
import { api, ApiError } from './api'
import type { Project, Task, TaskStatus } from './types'
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

  const refresh = () => {
    api.getProject(projectId).then(setProject)
    api.listTasks(projectId).then(setTasks)
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId])

  // Tell the server where we're looking so presence/viewer indicators stay
  // accurate as the user opens/closes the task panel.
  useEffect(() => {
    live.setViewing(projectId, selectedTaskId ?? undefined)
  }, [projectId, selectedTaskId])

  useWsEvents((evt) => {
    if (evt.projectId !== projectId) return
    if (evt.type.startsWith('task.') || evt.type === 'project.updated') {
      refresh()
    }
    if ((evt.type === 'task.updated' || evt.type === 'task.created') && evt.resourceId) {
      flash(evt.resourceId)
    }
  })

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
