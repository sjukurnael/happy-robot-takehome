import { useEffect, useMemo, useState } from 'react'
import { api } from './api'
import type { Project, ProjectStats } from './types'
import { useWsEvents, useAllProjectsPresence } from './useWsEvents'
import { Avatar } from './Avatar'
import { formatRelativeTime } from './format'

export function ProjectList({ onOpen }: { onOpen: (id: string) => void }) {
  const [projects, setProjects] = useState<Project[]>([])
  const [stats, setStats] = useState<Record<string, ProjectStats>>({})
  const [search, setSearch] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const presenceByProject = useAllProjectsPresence()

  // Two small requests, regardless of how many projects/tasks exist — the
  // stat cards used to be computed client-side by downloading every task
  // of every project, which re-shipped entire projects on each refresh.
  const refresh = async () => {
    const [projs, statList] = await Promise.all([api.listProjects(), api.listProjectStats()])
    setProjects(projs)
    setStats(Object.fromEntries(statList.map((s) => [s.projectId, s])))
  }

  useEffect(() => {
    refresh()
  }, [])

  useWsEvents((evt) => {
    // project.created / project.deleted are thin notifications outside the
    // per-project event log (see server/events.go); everything else
    // relevant here arrives wrapped as type "event".
    if (evt.type === 'project.created' || evt.type === 'project.deleted') {
      refresh()
      return
    }
    if (evt.type === 'event' && (evt.eventType === 'project.updated' || evt.eventType?.startsWith('task.'))) {
      refresh()
    }
  })

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    await api.createProject({ name, description, metadata: {} })
    setName('')
    setDescription('')
    refresh()
  }

  async function handleDelete(id: string, e: React.MouseEvent) {
    e.stopPropagation()
    if (!confirm('Delete this project and all its tasks?')) return
    await api.deleteProject(id)
    refresh()
  }

  const visibleProjects = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return projects
    return projects.filter(
      (p) => p.name.toLowerCase().includes(q) || p.description.toLowerCase().includes(q),
    )
  }, [projects, search])

  return (
    <div>
      <div className="projects-header-row">
        <h1>Projects</h1>
        <div className="spacer" />
        <input
          className="search-input"
          placeholder="Search projects…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <form onSubmit={handleCreate} className="row">
        <input placeholder="Project name" value={name} onChange={(e) => setName(e.target.value)} />
        <input placeholder="Description" value={description} onChange={(e) => setDescription(e.target.value)} />
        <button type="submit" className="btn-primary">
          Create project
        </button>
      </form>

      <div className="project-grid">
        {visibleProjects.map((p) => {
          const s = stats[p.id]
          const pct = s && s.total > 0 ? Math.round((s.done / s.total) * 100) : 0
          const online = presenceByProject[p.id]?.length ?? 0
          return (
            <div key={p.id} className="project-card" onClick={() => onOpen(p.id)}>
              <div className="project-card-top">
                <span className="project-icon">{p.name.trim()[0]?.toUpperCase() ?? '?'}</span>
                <div className="project-card-title">
                  <div className="project-name">{p.name}</div>
                  <div className="project-desc">{p.description}</div>
                </div>
                <button className="delete-btn" onClick={(e) => handleDelete(p.id, e)}>
                  Delete
                </button>
              </div>

              {s && (
                <>
                  <div className="progress-row">
                    <div className="progress-track">
                      <div className="progress-fill" style={{ width: `${pct}%` }} />
                    </div>
                    <span className="progress-label">
                      {s.done}/{s.total} done
                    </span>
                  </div>
                  <div className="project-meta-row">
                    {s.blocked > 0 && <span className="blocked-pill">{s.blocked} blocked</span>}
                    <span className="spacer" />
                    {s.assignees.length > 0 && (
                      <div className="avatar-stack">
                        {s.assignees.slice(0, 3).map((a) => (
                          <Avatar key={a} name={a} size={20} />
                        ))}
                      </div>
                    )}
                    <span className="meta-text">
                      {online > 0 && `${online} online · `}
                      edited {formatRelativeTime(s.lastEdited)}
                    </span>
                  </div>
                </>
              )}
            </div>
          )
        })}
        {visibleProjects.length === 0 && <p className="empty-hint">No projects yet — create one above.</p>}
      </div>
    </div>
  )
}
