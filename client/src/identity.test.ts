import { describe, expect, it } from 'vitest'
import { colorForClientId, initialsFor } from './identity'

describe('initialsFor', () => {
  it('takes first and last initials from a multi-word name', () => {
    expect(initialsFor('Maya Torres')).toBe('MT')
    expect(initialsFor('Ana de la Cruz')).toBe('AC')
  })

  it('takes a single initial from a one-word name', () => {
    expect(initialsFor('ana@co.com')).toBe('A')
    expect(initialsFor('bob')).toBe('B')
  })

  it('falls back to "?" for empty or whitespace-only names', () => {
    expect(initialsFor('')).toBe('?')
    expect(initialsFor('   ')).toBe('?')
  })
})

describe('colorForClientId', () => {
  it('is deterministic: same input, same color', () => {
    expect(colorForClientId('client_abc')).toBe(colorForClientId('client_abc'))
  })

  it('always yields a valid hsl() string with hue in [0, 360)', () => {
    for (const id of ['a', 'client_123', 'Maya Torres', '✨']) {
      const m = colorForClientId(id).match(/^hsl\((\d+), 65%, 45%\)$/)
      expect(m, `color for ${id}`).not.toBeNull()
      expect(Number(m![1])).toBeGreaterThanOrEqual(0)
      expect(Number(m![1])).toBeLessThan(360)
    }
  })

  it('distinguishes two collaborators who share a display name', () => {
    // The color is keyed by clientId, not name — two "Guest"s with
    // different clientIds must not collide.
    expect(colorForClientId('client_1')).not.toBe(colorForClientId('client_2'))
  })
})
