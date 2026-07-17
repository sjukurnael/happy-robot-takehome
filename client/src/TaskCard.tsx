import { useDraggable } from '@dnd-kit/core'
import type { PresenceEntry, Task } from './types'
import { Avatar } from './Avatar'

export function TaskCard({
  task,
  blocked,
  viewers,
  flash,
  onOpen,
}: {
  task: Task
  blocked: boolean
  viewers: PresenceEntry[]
  flash: boolean
  onOpen: () => void
}) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: task.id })
  const style = transform
    ? { transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` }
    : undefined
  const viewer = viewers[0]

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`task-card${isDragging ? ' dragging' : ''}${flash ? ' flash' : ''}`}
      onClick={onOpen}
      {...listeners}
      {...attributes}
    >
      {viewer && (
        <span className="viewing-badge">
          <Avatar name={viewer.name} size={16} className="pulse" />
          {viewer.name} viewing
        </span>
      )}

      <div className="task-card-top">{task.title}</div>

      <div className="tag-row">
        {task.configuration.priority && (
          <span className={`priority-badge priority-${task.configuration.priority}`}>
            {task.configuration.priority}
          </span>
        )}
        {blocked && <span className="blocked-badge">⊘ Blocked</span>}
        {task.configuration.tags.map((tag) => (
          <span key={tag} className="tag">
            {tag}
          </span>
        ))}
      </div>

      {task.assignedTo.length > 0 && (
        <div className="task-card-bottom">
          <div className="avatar-stack">
            {task.assignedTo.map((name) => (
              <Avatar key={name} name={name} size={22} />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
