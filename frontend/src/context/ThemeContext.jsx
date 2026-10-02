import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react'
import { accountApi } from '../services/api'
import { useAuth } from './AuthContext'

const ThemeContext = createContext(null)
const STORAGE_KEY = 'flashcart_theme'

export const THEMES = [
  { id: 'light', name: 'Light', detail: 'Clean, bright surfaces', motion: 'Still' },
  { id: 'dark', name: 'Dark', detail: 'Low-light contrast', motion: 'Still' },
  { id: 'gradient', name: 'Gradient', detail: 'Flowing color wash', motion: 'Soft motion' },
  { id: 'aurora', name: 'Aurora', detail: 'Cool northern glow', motion: 'Animated' },
  { id: 'neon', name: 'Neon', detail: 'Electric accent contrast', motion: 'Animated' },
  { id: 'particles', name: 'Particles', detail: 'Subtle floating lights', motion: 'Ambient' },
]

const isValidTheme = (value) => THEMES.some((theme) => theme.id === value)

function readTheme() {
  try {
    const value = localStorage.getItem(STORAGE_KEY)
    return isValidTheme(value) ? value : 'light'
  } catch {
    return 'light'
  }
}

function applyThemeToDocument(theme) {
  const root = document.documentElement
  root.dataset.theme = theme
  root.classList.remove('theme-transition')
  void root.offsetWidth
  root.classList.add('theme-transition')
  window.setTimeout(() => root.classList.remove('theme-transition'), 850)
}

export function ThemeProvider({ children }) {
  const { user, isAuthenticated, updateThemePreference } = useAuth()
  const [theme, setTheme] = useState(readTheme)
  const [preview, setPreview] = useState(null)

  useEffect(() => {
    applyThemeToDocument(preview || theme)
  }, [theme, preview])

  useEffect(() => {
    if (!isAuthenticated || !user?.id) return undefined
    let active = true
    accountApi.theme()
      .then(({ data }) => {
        if (!active || !isValidTheme(data.theme)) return
        setTheme(data.theme)
        setPreview(null)
        localStorage.setItem(STORAGE_KEY, data.theme)
        updateThemePreference(data.theme)
      })
      .catch(() => {})
    return () => { active = false }
  }, [isAuthenticated, user?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const previewTheme = useCallback((value) => {
    if (isValidTheme(value)) setPreview(value)
  }, [])

  const cancelPreview = useCallback(() => setPreview(null), [])

  const commitTheme = useCallback(async (value) => {
    if (!isValidTheme(value)) throw new Error('Choose a valid theme.')
    if (isAuthenticated) await accountApi.saveTheme(value)
    localStorage.setItem(STORAGE_KEY, value)
    setTheme(value)
    setPreview(null)
    updateThemePreference(value)
  }, [isAuthenticated, updateThemePreference])

  const value = useMemo(() => ({ theme, preview, themes: THEMES, previewTheme, cancelPreview, commitTheme }), [theme, preview, previewTheme, cancelPreview, commitTheme])
  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export const useTheme = () => useContext(ThemeContext)