import { useEffect, useState } from 'react'
import { api } from './api'
import type { Project } from './types'
import { useWsEvents } from './useWsEvents'

export function ProjectList({ onOpen }: { onOpen: (id: string) => void }) {
  const [projects, setProjects] = useState<Project[]>([])
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')

  const refresh = () => api.listProjects().then(setProjects)

  useEffect(() => {
    refresh()
  }, [])

  useWsEvents((evt) => {
    if (evt.type === 'project.updated' || evt.type === 'project.deleted') {
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

  return (
    <div>
      <h1>Projects</h1>
      <form onSubmit={handleCreate} className="row">
        <input placeholder="Project name" value={name} onChange={(e) => setName(e.target.value)} />
        <input placeholder="Description" value={description} onChange={(e) => setDescription(e.target.value)} />
        <button type="submit">Create project</button>
      </form>

      <ul className="list">
        {projects.map((p) => (
          <li key={p.id} className="card" onClick={() => onOpen(p.id)}>
            <div>
              <strong>{p.name}</strong>
              <p>{p.description}</p>
            </div>
            <button onClick={(e) => handleDelete(p.id, e)}>Delete</button>
          </li>
        ))}
        {projects.length === 0 && <p>No projects yet — create one above.</p>}
      </ul>
    </div>
  )
}
