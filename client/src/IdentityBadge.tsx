import { useState } from 'react'
import { colorForClientId, renameIdentity } from './identity'
import { live } from './live'

export function IdentityBadge() {
  const [name, setName] = useState(live.me.name)

  function handleRename() {
    const next = window.prompt('Change display name', name)
    if (!next?.trim()) return
    const identity = renameIdentity(next)
    live.me.name = identity.name // keep the shared singleton in sync for comment authorship etc.
    setName(identity.name)
    live.rename(identity.name)
  }

  return (
    <div className="identity-badge" onClick={handleRename} title="Click to change your name">
      <span className="dot" style={{ background: colorForClientId(live.me.clientId) }} />
      {name}
    </div>
  )
}
