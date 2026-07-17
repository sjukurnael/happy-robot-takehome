import { useEffect, useState } from 'react'

const KEY = 'taskman.theme'
type Theme = 'light' | 'dark'

function loadTheme(): Theme | null {
  const stored = localStorage.getItem(KEY)
  return stored === 'light' || stored === 'dark' ? stored : null
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme | null>(loadTheme)

  useEffect(() => {
    if (theme) {
      document.documentElement.setAttribute('data-theme', theme)
      localStorage.setItem(KEY, theme)
    } else {
      document.documentElement.removeAttribute('data-theme')
    }
  }, [theme])

  const isDark = theme
    ? theme === 'dark'
    : window.matchMedia?.('(prefers-color-scheme: dark)').matches

  return (
    <button
      type="button"
      className="theme-toggle"
      onClick={() => setTheme(isDark ? 'light' : 'dark')}
      title={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
      aria-label="Toggle color theme"
    >
      {isDark ? '☀' : '☾'}
    </button>
  )
}
