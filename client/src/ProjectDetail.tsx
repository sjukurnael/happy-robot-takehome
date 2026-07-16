import { useEffect, useState } from 'react'
import { api } from './api'
import type { Project, Task, TaskStatus } from './types'
import { useWsEvents, usePresence } from './useWsEvents'
import { PresenceRoster, viewersOfTask } from './Presence'
import { colorForClientId } from './identity'

const STATUSES: TaskStatus[] = ['todo', 'in_progress', 'done']

export function ProjectDetail({
  projectId,
  onBack,
  onOpenTask,
}: {
  projectId: string
  onBack: () => void
  onOpenTask: (taskId: string) => void
}) {
  const [project, setProject] = useState<Project | null>(null)
  const [tasks, setTasks] = useState<Task[]>([])
  const [title, setTitle] = useState('')
  const [priority, setPriority] = useState('medium')
  const presence = usePresence(projectId)

  const refresh = () => {
    api.getProject(projectId).then(setProject)
    api.listTasks(projectId).then(setTasks)
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId])

  useWsEvents((evt) => {
    if (evt.projectId !== projectId) return
    if (evt.type.startsWith('task.') || evt.type === 'project.updated') {
      refresh()
    }
  })

  async function handleCreateTask(e: React.FormEvent) {
    e.preventDefault()
    if (!title.trim()) return
    await api.createTask(projectId, { title, configuration: { priority, tags: [] } })
    setTitle('')
    refresh()
  }

  async function handleStatusChange(taskId: string, status: TaskStatus) {
    await api.updateTask(taskId, { status })
    refresh()
  }

  async function handleDeleteTask(taskId: string, e: React.MouseEvent) {
    e.stopPropagation()
    await api.deleteTask(taskId)
    refresh()
  }

  if (!project) return <p>Loading…</p>

  return (
    <div>
      <button onClick={onBack}>&larr; Projects</button>
      <h1>{project.name}</h1>
      <p>{project.description}</p>
      <PresenceRoster entries={presence} />

      <form onSubmit={handleCreateTask} className="row">
        <input placeholder="Task title" value={title} onChange={(e) => setTitle(e.target.value)} />
        <select value={priority} onChange={(e) => setPriority(e.target.value)}>
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
        </select>
        <button type="submit">Add task</button>
      </form>

      <ul className="list">
        {tasks.map((t) => {
          const viewers = viewersOfTask(presence, t.id)
          return (
            <li key={t.id} className="card" onClick={() => onOpenTask(t.id)}>
              <div>
                <strong>{t.title}</strong>
                <p>priority: {t.configuration?.priority || 'n/a'}</p>
              </div>
              <div className="task-controls" onClick={(e) => e.stopPropagation()}>
                {viewers.length > 0 && (
                  <span className="viewer-dots" title={viewers.map((v) => v.name).join(', ')}>
                    {viewers.map((v) => (
                      <span key={v.clientId} className="dot" style={{ background: colorForClientId(v.clientId) }} />
                    ))}
                  </span>
                )}
                <select value={t.status} onChange={(e) => handleStatusChange(t.id, e.target.value as TaskStatus)}>
                  {STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
                <button onClick={(e) => handleDeleteTask(t.id, e)}>Delete</button>
              </div>
            </li>
          )
        })}
        {tasks.length === 0 && <p>No tasks yet — add one above.</p>}
      </ul>
    </div>
  )
}
