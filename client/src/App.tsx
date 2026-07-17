import { useEffect, useState } from 'react'
import { ProjectList } from './ProjectList'
import { ProjectDetail } from './ProjectDetail'
import { IdentityBadge } from './IdentityBadge'
import { ThemeToggle } from './ThemeToggle'
import { live } from './live'
import './App.css'

type View = { name: 'projects' } | { name: 'project'; projectId: string }

function App() {
  const [view, setView] = useState<View>({ name: 'projects' })

  // ProjectDetail owns presence viewing state while it's mounted (including
  // which task's panel is open); this only needs to clear it when we leave
  // the project entirely.
  useEffect(() => {
    if (view.name !== 'project') live.setViewing('')
  }, [view])

  return (
    <div className="app">
      {view.name === 'projects' && (
        <>
          <div className="topbar">
            <ThemeToggle />
            <IdentityBadge />
          </div>
          <ProjectList onOpen={(projectId) => setView({ name: 'project', projectId })} />
        </>
      )}
      {view.name === 'project' && (
        <ProjectDetail projectId={view.projectId} onBack={() => setView({ name: 'projects' })} />
      )}
    </div>
  )
}

export default App
