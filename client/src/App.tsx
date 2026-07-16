import { useEffect, useState } from 'react'
import { ProjectList } from './ProjectList'
import { ProjectDetail } from './ProjectDetail'
import { TaskDetail } from './TaskDetail'
import { IdentityBadge } from './IdentityBadge'
import { live } from './live'
import './App.css'

type View =
  | { name: 'projects' }
  | { name: 'project'; projectId: string }
  | { name: 'task'; projectId: string; taskId: string }

function App() {
  const [view, setView] = useState<View>({ name: 'projects' })

  // Single source of truth for "where is this client looking" — every
  // view transition tells the server, which is all presence needs.
  useEffect(() => {
    if (view.name === 'project') live.setViewing(view.projectId)
    else if (view.name === 'task') live.setViewing(view.projectId, view.taskId)
    else live.setViewing('')
  }, [view])

  return (
    <div className="app">
      <IdentityBadge />
      {view.name === 'projects' && (
        <ProjectList onOpen={(projectId) => setView({ name: 'project', projectId })} />
      )}
      {view.name === 'project' && (
        <ProjectDetail
          projectId={view.projectId}
          onBack={() => setView({ name: 'projects' })}
          onOpenTask={(taskId) => setView({ name: 'task', projectId: view.projectId, taskId })}
        />
      )}
      {view.name === 'task' && (
        <TaskDetail
          taskId={view.taskId}
          onBack={() => setView({ name: 'project', projectId: view.projectId })}
        />
      )}
    </div>
  )
}

export default App
