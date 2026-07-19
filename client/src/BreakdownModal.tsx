import { useEffect, useState } from 'react'
import { api, ApiError } from './api'
import type { BreakdownSuggestion, Task } from './types'
import { remapSelectedSuggestions } from './taskUtils'

// Review dialog for the AI task breakdown. Phase 1 (on mount) fetches
// suggestions — nothing is persisted, so closing at any point abandons
// cleanly. Phase 2 submits only the checked suggestions; the board updates
// through the resulting task.created / task.dependencies_changed events,
// so no callbacks beyond onClose are needed.
export function BreakdownModal({ task, onClose }: { task: Task; onClose: () => void }) {
  const [suggestions, setSuggestions] = useState<BreakdownSuggestion[] | null>(null)
  const [selected, setSelected] = useState<boolean[]>([])
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    setError(null)
    setSuggestions(null)
    api
      .breakdownTask(task.id)
      .then((res) => {
        if (cancelled) return
        setSuggestions(res.suggestions)
        setSelected(res.suggestions.map(() => true))
      })
      .catch((err) => {
        if (cancelled) return
        setError(err instanceof ApiError ? err.message : 'Failed to get suggestions')
      })
    return () => {
      cancelled = true
    }
  }, [task.id, attempt])

  const selectedCount = selected.filter(Boolean).length

  function toggle(i: number) {
    setSelected((prev) => prev.map((v, j) => (j === i ? !v : v)))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!suggestions || selectedCount === 0) return
    setSubmitting(true)
    setError(null)
    try {
      await api.applyBreakdown(task.id, remapSelectedSuggestions(suggestions, selected))
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create subtasks')
      setSubmitting(false)
    }
  }

  return (
    <div
      className="modal-overlay"
      onClick={(e) => {
        e.stopPropagation()
        onClose()
      }}
    >
      <form className="modal breakdown-modal" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <div className="modal-header-row">
          <h2>✨ Break down “{task.title}”</h2>
          <button type="button" className="panel-close" onClick={onClose} aria-label="Close">
            &times;
          </button>
        </div>

        {error && (
          <div className="error-banner breakdown-error">
            <p>{error}</p>
            {!suggestions && (
              <button type="button" className="btn-secondary" onClick={() => setAttempt((a) => a + 1)}>
                Retry
              </button>
            )}
          </div>
        )}

        {!suggestions && !error && (
          <p className="muted breakdown-loading">
            Asking Claude to break this down… this can take up to a minute. Nothing is created until you
            confirm.
          </p>
        )}

        {suggestions && (
          <>
            <p className="muted">
              Uncheck anything you don’t want. “{task.title}” will depend on the new subtasks, so it can’t be
              completed until they’re done.
            </p>
            <div className="breakdown-list">
              {suggestions.map((s, i) => (
                <label key={i} className={`breakdown-row${selected[i] ? '' : ' deselected'}`}>
                  <input type="checkbox" checked={selected[i]} onChange={() => toggle(i)} />
                  <div className="breakdown-row-body">
                    <div className="breakdown-row-head">
                      <strong>{s.title}</strong>
                      <span className={`priority-select priority-${s.priority} breakdown-priority`}>
                        {s.priority}
                      </span>
                      {s.tags.map((t) => (
                        <span key={t} className="tag">
                          {t}
                        </span>
                      ))}
                    </div>
                    {s.description && <p className="muted">{s.description}</p>}
                    {s.dependsOn.length > 0 && (
                      <p className="breakdown-deps">
                        after:{' '}
                        {s.dependsOn.map((d) => (
                          <span key={d} className="dep-kind">
                            {suggestions[d].title}
                          </span>
                        ))}
                      </p>
                    )}
                  </div>
                </label>
              ))}
            </div>
          </>
        )}

        <div className="modal-actions">
          <button type="button" className="btn-secondary" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="btn-primary" disabled={!suggestions || submitting || selectedCount === 0}>
            {submitting ? 'Creating…' : `Create ${selectedCount} subtask${selectedCount === 1 ? '' : 's'}`}
          </button>
        </div>
      </form>
    </div>
  )
}
