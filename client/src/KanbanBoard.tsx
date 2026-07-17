import { DndContext, PointerSensor, useDroppable, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import type { PresenceEntry, Task, TaskStatus } from './types'
import { TaskCard } from './TaskCard'
import { viewersOfTask } from './Presence'
import { isTaskBlocked } from './taskUtils'

const COLUMNS: { status: TaskStatus; label: string }[] = [
  { status: 'todo', label: 'To Do' },
  { status: 'in_progress', label: 'In Progress' },
  { status: 'done', label: 'Done' },
]

export function KanbanBoard({
  tasks,
  presence,
  highlighted,
  onMoveTask,
  onOpenTask,
}: {
  tasks: Task[]
  presence: PresenceEntry[]
  highlighted: Set<string>
  onMoveTask: (taskId: string, status: TaskStatus) => void
  onOpenTask: (taskId: string) => void
}) {
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const tasksById = new Map(tasks.map((t) => [t.id, t]))

  function handleDragEnd(event: DragEndEvent) {
    const taskId = String(event.active.id)
    const newStatus = event.over?.id as TaskStatus | undefined
    const task = tasksById.get(taskId)
    if (!newStatus || !task || task.status === newStatus) return
    onMoveTask(taskId, newStatus)
  }

  return (
    <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
      <div className="board">
        {COLUMNS.map((col) => (
          <Column
            key={col.status}
            status={col.status}
            label={col.label}
            tasks={tasks.filter((t) => t.status === col.status)}
            presence={presence}
            highlighted={highlighted}
            tasksById={tasksById}
            onOpenTask={onOpenTask}
          />
        ))}
      </div>
    </DndContext>
  )
}

function Column({
  status,
  label,
  tasks,
  presence,
  highlighted,
  tasksById,
  onOpenTask,
}: {
  status: TaskStatus
  label: string
  tasks: Task[]
  presence: PresenceEntry[]
  highlighted: Set<string>
  tasksById: Map<string, Task>
  onOpenTask: (taskId: string) => void
}) {
  const { setNodeRef, isOver } = useDroppable({ id: status })

  return (
    <div ref={setNodeRef} className={`column${isOver ? ' column-over' : ''}`}>
      <div className="column-head">
        <span className={`status-dot status-dot-${status}`} />
        <span className="column-label">{label}</span>
        <span className="count">{tasks.length}</span>
      </div>
      <div className="column-body">
        {tasks.map((t) => (
          <TaskCard
            key={t.id}
            task={t}
            blocked={isTaskBlocked(t, tasksById)}
            viewers={viewersOfTask(presence, t.id)}
            flash={highlighted.has(t.id)}
            onOpen={() => onOpenTask(t.id)}
          />
        ))}
        {tasks.length === 0 && <p className="empty-hint">No tasks</p>}
      </div>
    </div>
  )
}
