import { useEffect, useState } from 'react'
import { api } from './api'
import type { Comment, Task } from './types'
import { useWsEvents, usePresence } from './useWsEvents'
import { PresenceRoster, viewersOfTask } from './Presence'
import { live } from './live'

export function TaskDetail({ taskId, onBack }: { taskId: string; onBack: () => void }) {
  const [task, setTask] = useState<Task | null>(null)
  const [comments, setComments] = useState<Comment[]>([])
  const [content, setContent] = useState('')
  const presence = usePresence(task?.projectId ?? null)

  const refresh = () => {
    api.getTask(taskId).then(setTask)
    api.listComments(taskId).then(setComments)
  }

  useEffect(() => {
    refresh()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [taskId])

  useWsEvents((evt) => {
    if (evt.type === 'comment.created' || evt.type === 'task.updated') {
      refresh()
    }
  })

  async function handleAddComment(e: React.FormEvent) {
    e.preventDefault()
    if (!content.trim()) return
    await api.createComment(taskId, { content, author: live.me.name })
    setContent('')
    refresh()
  }

  if (!task) return <p>Loading…</p>

  const viewers = viewersOfTask(presence, taskId)

  return (
    <div>
      <button onClick={onBack}>&larr; Back to project</button>
      <h1>{task.title}</h1>
      <p>status: {task.status}</p>
      <PresenceRoster entries={viewers} />

      <h2>Comments</h2>
      <ul className="list">
        {comments.map((c) => (
          <li key={c.id} className="comment">
            <strong>{c.author}</strong>
            <span className="ts">{new Date(c.timestamp).toLocaleString()}</span>
            <p>{c.content}</p>
          </li>
        ))}
        {comments.length === 0 && <p>No comments yet.</p>}
      </ul>

      <form onSubmit={handleAddComment} className="row">
        <input placeholder={`Comment as ${live.me.name}…`} value={content} onChange={(e) => setContent(e.target.value)} />
        <button type="submit">Comment</button>
      </form>
    </div>
  )
}
