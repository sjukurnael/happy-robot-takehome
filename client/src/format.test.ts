import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { formatRelativeTime } from './format'

const NOW = new Date('2026-07-19T12:00:00Z')

function isoSecondsAgo(sec: number): string {
  return new Date(NOW.getTime() - sec * 1000).toISOString()
}

describe('formatRelativeTime', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders sub-minute ages as "just now"', () => {
    expect(formatRelativeTime(isoSecondsAgo(0))).toBe('just now')
    expect(formatRelativeTime(isoSecondsAgo(59))).toBe('just now')
  })

  it('clamps future timestamps (clock skew) to "just now" instead of negatives', () => {
    expect(formatRelativeTime(isoSecondsAgo(-120))).toBe('just now')
  })

  it('rolls through each unit at its boundary', () => {
    expect(formatRelativeTime(isoSecondsAgo(60))).toBe('1m ago')
    expect(formatRelativeTime(isoSecondsAgo(59 * 60))).toBe('59m ago')
    expect(formatRelativeTime(isoSecondsAgo(60 * 60))).toBe('1h ago')
    expect(formatRelativeTime(isoSecondsAgo(23 * 3600))).toBe('23h ago')
    expect(formatRelativeTime(isoSecondsAgo(24 * 3600))).toBe('1d ago')
    expect(formatRelativeTime(isoSecondsAgo(29 * 86400))).toBe('29d ago')
    expect(formatRelativeTime(isoSecondsAgo(30 * 86400))).toBe('1mo ago')
    expect(formatRelativeTime(isoSecondsAgo(11 * 30 * 86400))).toBe('11mo ago')
    expect(formatRelativeTime(isoSecondsAgo(12 * 30 * 86400))).toBe('1y ago')
  })
})
