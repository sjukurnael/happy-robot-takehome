import { useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { Comment, PresenceEntry, Task, TaskStatus } from './types'
import { useWsEvents } from './useWsEvents'
import { viewersOfTask } from './Presence'
import { Avatar } from './Avatar'
import { formatRelativeTime } from './format'
import { live } from './live'

const PRIORITIES = ['low', 'medium', 'high']
const STATUSES: TaskStatus[] = ['todo', 'in_progress', 'done']
const STATUS_LABEL: Record<TaskStatus, string> = {
  todo: 'To Do',
  in_progress: 'In Progress',
  done: 'Done',
}

export function TaskPanel({
  taskId,
  tasks,
  presence,
  onClose,
}: {
  taskId: string
  tasks: Task[]
  presence: PresenceEntry[]
  onClose: () => void
}) {
  const task = tasks.find((t) => t.id === taskId) ?? null
  const watchers = viewersOfTask(presence, taskId)

  const [comments, setComments] = useState<Comment[]>([])
  const [commentContent, setCommentContent] = useState('')
  const [title, setTitle] = useState('')
  const [priority, setPriority] = useState('medium')
  const [description, setDescription] = useState('')
  const [tags, setTags] = useState('')
  const [assignees, setAssignees] = useState('')
  const [depPickerOpen, setDepPickerOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Reseed local draft fields only when switching to a different task —
  // not on every WS-driven refresh of `tasks` — so an in-flight edit in
  // this panel isn't clobbered by someone else's unrelated update.
  useEffect(() => {
    if (!task) return
    setTitle(task.title)
    setPriority(task.configuration.priority || 'medium')
    setDescription(task.configuration.description || '')
    setTags(task.configuration.tags.join(', '))
    setAssignees(task.assignedTo.join(', '))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId])

  const refreshComments = () => api.listComments(taskId).then(setComments)

  useEffect(() => {
    refreshComments()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId])

  useWsEvents((evt) => {
    if (evt.type === 'comment.created' && task && evt.projectId === task.projectId) {
      refreshComments()
    }
  })

  if (!task) return null

  async function saveField(patch: Parameters<typeof api.updateTask>[1]) {
    setError(null)
    try {
      await api.updateTask(taskId, patch)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Update failed')
    }
  }

  function commitTitle() {
    if (title.trim() && title !== task!.title) saveField({ title: title.trim() })
  }

  function commitConfiguration(nextPriority = priority, nextDescription = description, nextTags = tags) {
    saveField({
      configuration: {
        ...task!.configuration,
        priority: nextPriority,
        description: nextDescription,
        tags: nextTags
          .split(',')
          .map((t) => t.trim())
          .filter(Boolean),
      },
    })
  }

  function commitAssignees() {
    saveField({
      assignedTo: assignees
        .split(',')
        .map((a) => a.trim())
        .filter(Boolean),
    })
  }

  function removeDependency(depId: string) {
    saveField({ dependencies: task!.dependencies.filter((d) => d !== depId) })
  }

  function addDependency(depId: string) {
    saveField({ dependencies: [...task!.dependencies, depId] })
    setDepPickerOpen(false)
  }

  async function handleAddComment(e: React.FormEvent) {
    e.preventDefault()
    if (!commentContent.trim()) return
    await api.createComment(taskId, { content: commentContent, author: live.me.name })
    setCommentContent('')
    refreshComments()
  }

  async function handleDeleteComment(id: string) {
    await api.deleteComment(id)
    refreshComments()
  }

  async function handleDeleteTask() {
    if (!confirm('Delete this task? This cannot be undone.')) return
    try {
      await api.deleteTask(taskId)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete task')
    }
  }

  const previewTags = tags
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)
  const previewAssignees = assignees
    .split(',')
    .map((a) => a.trim())
    .filter(Boolean)
  const depTasks = task.dependencies.map((id) => tasks.find((t) => t.id === id)).filter((t): t is Task => !!t)
  const candidateDeps = tasks.filter((t) => t.id !== task.id && !task.dependencies.includes(t.id))

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="panel" onClick={(e) => e.stopPropagation()}>
        <div className="panel-header">
          {watchers.length > 0 && (
            <span className="watching-row">
              watching:
              {watchers.map((w) => (
                <Avatar key={w.clientId} name={w.name} size={20} className="pulse" />
              ))}
            </span>
          )}
          <div className="panel-header-actions">
            <button type="button" className="delete-btn" onClick={handleDeleteTask}>
              Delete
            </button>
            <button type="button" className="panel-close" onClick={onClose} aria-label="Close">
              &times;
            </button>
          </div>
        </div>

        {error && <p className="error-banner">{error}</p>}

        <input
          className="panel-title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          onBlur={commitTitle}
        />

        <div className="panel-grid">
          <span className="panel-grid-label">Status</span>
          <select
            className={`status-select status-${task.status}`}
            value={task.status}
            onChange={(e) => saveField({ status: e.target.value as TaskStatus })}
          >
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABEL[s]}
              </option>
            ))}
          </select>

          <span className="panel-grid-label">Priority</span>
          <select
            className={`priority-select priority-${priority}`}
            value={priority}
            onChange={(e) => {
              setPriority(e.target.value)
              commitConfiguration(e.target.value, description, tags)
            }}
          >
            {PRIORITIES.map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>

          <span className="panel-grid-label">Assignees</span>
          <div className="assignee-edit">
            {previewAssignees.length > 0 && (
              <div className="avatar-stack">
                {previewAssignees.map((a) => (
                  <Avatar key={a} name={a} size={22} />
                ))}
              </div>
            )}
            <input
              value={assignees}
              onChange={(e) => setAssignees(e.target.value)}
              onBlur={commitAssignees}
              placeholder="comma separated"
            />
          </div>

          <span className="panel-grid-label">Tags</span>
          <div className="tag-edit">
            {previewTags.length > 0 && (
              <div className="tag-row">
                {previewTags.map((t) => (
                  <span key={t} className="tag">
                    {t}
                  </span>
                ))}
              </div>
            )}
            <input
              value={tags}
              onChange={(e) => setTags(e.target.value)}
              onBlur={() => commitConfiguration()}
              placeholder="comma separated"
            />
          </div>
        </div>

        <label className="field">
          Description
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onBlur={() => commitConfiguration()}
            rows={3}
          />
        </label>

        <div className="field">
          <div className="field-label-row">
            <span>Dependencies</span>
            <button type="button" onClick={() => setDepPickerOpen((v) => !v)}>
              + add
            </button>
          </div>
          <div className="dep-rows">
            {depTasks.map((d) => (
              <div key={d.id} className="dep-row">
                <span className="dep-kind">BLOCKED BY</span>
                <span className={`status-dot status-dot-${d.status}`} />
                <span className="dep-name">{d.title}</span>
                <span className="spacer" />
                <span className="dep-status">{STATUS_LABEL[d.status]}</span>
                <button type="button" onClick={() => removeDependency(d.id)} aria-label={`Remove dependency ${d.title}`}>
                  &times;
                </button>
              </div>
            ))}
            {depTasks.length === 0 && <span className="muted">None</span>}
          </div>
          {depPickerOpen && (
            <div className="dep-picker">
              {candidateDeps.length === 0 && <p className="muted">No other tasks to depend on.</p>}
              {candidateDeps.map((t) => (
                <button type="button" key={t.id} className="dep-picker-option" onClick={() => addDependency(t.id)}>
                  {t.title} <span className={`status-badge status-${t.status}`}>{STATUS_LABEL[t.status]}</span>
                </button>
              ))}
            </div>
          )}
        </div>

        <h3>Comments · {comments.length}</h3>
        <ul className="list">
          {comments.map((c) => (
            <li key={c.id} className="comment">
              <Avatar name={c.author} size={24} />
              <div className="comment-body">
                <div className="comment-head">
                  <strong>{c.author}</strong>
                  <span className="ts">{formatRelativeTime(c.timestamp)}</span>
                  <button
                    type="button"
                    className="comment-delete"
                    onClick={() => handleDeleteComment(c.id)}
                    aria-label="Delete comment"
                  >
                    &times;
                  </button>
                </div>
                <p>{c.content}</p>
              </div>
            </li>
          ))}
          {comments.length === 0 && <p className="muted">No comments yet.</p>}
        </ul>
        <form onSubmit={handleAddComment} className="row">
          <input
            placeholder={`Comment as ${live.me.name}…`}
            value={commentContent}
            onChange={(e) => setCommentContent(e.target.value)}
          />
          <button type="submit" className="btn-primary">
            Send
          </button>
        </form>
      </div>
    </div>
  )
}
