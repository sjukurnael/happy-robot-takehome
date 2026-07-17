import { useState } from 'react'
import { api, ApiError } from './api'
import type { Task, TaskStatus } from './types'

const PRIORITIES = ['low', 'medium', 'high']
const STATUS_LABEL: Record<TaskStatus, string> = {
  todo: 'To Do',
  in_progress: 'In Progress',
  done: 'Done',
}

export function NewTaskForm({
  projectId,
  tasks,
  onClose,
  onCreated,
}: {
  projectId: string
  tasks: Task[]
  onClose: () => void
  onCreated: () => void
}) {
  const [title, setTitle] = useState('')
  const [priority, setPriority] = useState('medium')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [dependencies, setDependencies] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  function toggleDependency(id: string) {
    setDependencies((deps) => (deps.includes(id) ? deps.filter((d) => d !== id) : [...deps, id]))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!title.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      await api.createTask(projectId, {
        title: title.trim(),
        configuration: {
          priority,
          description,
          tags: tags
            .split(',')
            .map((t) => t.trim())
            .filter(Boolean),
        },
        dependencies,
      })
      onCreated()
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create task')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="modal-overlay" onClick={onClose}>
      <form className="modal" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <div className="modal-header-row">
          <h2>New task</h2>
          <button type="button" className="panel-close" onClick={onClose} aria-label="Close">
            &times;
          </button>
        </div>
        {error && <p className="error-banner">{error}</p>}

        <input
          className="panel-title"
          autoFocus
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Task title"
        />

        <div className="new-task-pill-row">
          <span className="status-select status-todo" style={{ pointerEvents: 'none' }}>
            {STATUS_LABEL.todo}
          </span>
          <select
            className={`priority-select priority-${priority}`}
            value={priority}
            onChange={(e) => setPriority(e.target.value)}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </div>

        <label className="field">
          Description
          <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} />
        </label>

        <label className="field">
          Tags (comma separated)
          <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="frontend, urgent" />
        </label>

        {tasks.length > 0 && (
          <div className="field">
            <div className="field-label-row">
              <span>Depends on</span>
            </div>
            <div className="dependency-picker">
              {tasks.map((t) => {
                const selected = dependencies.includes(t.id)
                return (
                  <button
                    type="button"
                    key={t.id}
                    className={`dep-toggle-chip${selected ? ' selected' : ''}`}
                    onClick={() => toggleDependency(t.id)}
                  >
                    {selected ? '✓ ' : ''}
                    {t.title}
                    <span className={`status-badge status-${t.status}`}>{STATUS_LABEL[t.status]}</span>
                  </button>
                )
              })}
            </div>
          </div>
        )}

        <div className="modal-actions">
          <button type="button" className="btn-secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={submitting || !title.trim()}>
            {submitting ? 'Creating…' : 'Create task'}
          </button>
        </div>
      </form>
    </div>
  )
}
