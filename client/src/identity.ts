const KEY = 'taskman.identity'

export interface Identity {
  clientId: string
  name: string
}

// sessionStorage, not localStorage: each tab gets its own identity,
// so opening two tabs naturally simulates two different collaborators
// instead of sharing one identity across them.
function load(): Identity | null {
  const raw = sessionStorage.getItem(KEY)
  if (!raw) return null
  try {
    return JSON.parse(raw) as Identity
  } catch {
    return null
  }
}

function save(identity: Identity) {
  sessionStorage.setItem(KEY, JSON.stringify(identity))
}

export function getIdentity(): Identity {
  const existing = load()
  if (existing) return existing
  const name = window.prompt('Enter your display name', 'Guest')?.trim() || 'Guest'
  const identity: Identity = { clientId: crypto.randomUUID(), name }
  save(identity)
  return identity
}

export function renameIdentity(name: string): Identity {
  const identity = getIdentity()
  identity.name = name.trim() || identity.name
  save(identity)
  return identity
}

// Deterministic color from clientId so two collaborators who happen to
// share a display name are still visually distinguishable.
export function colorForClientId(clientId: string): string {
  let hash = 0
  for (let i = 0; i < clientId.length; i++) {
    hash = (hash * 31 + clientId.charCodeAt(i)) >>> 0
  }
  return `hsl(${hash % 360}, 65%, 45%)`
}
