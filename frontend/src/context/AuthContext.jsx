import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { authApi } from '../services/api'

const AuthContext = createContext(null)

const ACCESS = 'flashcart_access_token'
const REFRESH = 'flashcart_refresh_token'
const USER = 'flashcart_user'

// Idle timeout: if there's no mouse/keyboard/touch activity for this long
// while someone is logged in, we log them out client-side. The account
// itself is untouched — same email/password logs back in immediately.
//
// This used to be 5 minutes, which was aggressive enough that a shopper
// reading a couple of product pages or filling in a delivery address would
// get silently logged out — and because CartContext clears its in-memory
// items the moment isAuthenticated goes false, that looked exactly like
// "my cart got emptied". Cart data itself was never lost (it lives in
// Redis, keyed by user, with no TTL), but the user couldn't see it without
// logging back in. Raised to 30 minutes, which is a more realistic
// "actually walked away" signal for a shopping session.
const IDLE_TIMEOUT_MS = 30 * 60 * 1000
const ACTIVITY_EVENTS = ['mousemove', 'keydown', 'mousedown', 'touchstart', 'scroll']

function readUser() {
  try {
    return JSON.parse(localStorage.getItem(USER) || 'null')
  } catch {
    return null
  }
}

export function AuthProvider({ children }) {
  const [user, setUser] = useState(readUser)
  const [sessionExpired, setSessionExpired] = useState(false)
  const [welcomeMessage, setWelcomeMessage] = useState(null)
  const idleTimer = useRef(null)

  const save = (data) => {
    localStorage.setItem(ACCESS, data.access_token)
    if (data.refresh_token) localStorage.setItem(REFRESH, data.refresh_token)
    const next = { id: data.user_id, role: data.role || 'USER' }
    localStorage.setItem(USER, JSON.stringify(next))
    setUser(next)
    setSessionExpired(false)
  }

  const login = async (body) => {
    const { data } = await authApi.login(body)
    save(data)
    return data
  }

  const register = async (body) => {
    const { data } = await authApi.register(body)
    save(data)
    if (data.welcome_message) setWelcomeMessage(data.welcome_message)
    return data
  }

  const clearWelcomeMessage = () => setWelcomeMessage(null)

  const logout = (reason) => {
    ;[ACCESS, REFRESH, USER].forEach((k) => localStorage.removeItem(k))
    setUser(null)
    if (reason === 'idle') setSessionExpired(true)
  }

  const dismissSessionExpired = () => setSessionExpired(false)

  const updateThemePreference = useCallback((theme) => {
    setUser((current) => {
      if (!current) return current
      const next = { ...current, theme }
      localStorage.setItem(USER, JSON.stringify(next))
      return next
    })
  }, [])

  const isAdmin = user?.role === 'ADMIN'

  // Idle auto-logout: only runs while someone is logged in. Any activity
  // event resets the clock; if it ever fires, we log out and flag the
  // reason so the login screen can say "session expired, please log back in".
  useEffect(() => {
    if (!user) return undefined

    const resetTimer = () => {
      if (idleTimer.current) clearTimeout(idleTimer.current)
      idleTimer.current = setTimeout(() => logout('idle'), IDLE_TIMEOUT_MS)
    }

    resetTimer()
    ACTIVITY_EVENTS.forEach((evt) => window.addEventListener(evt, resetTimer))

    return () => {
      if (idleTimer.current) clearTimeout(idleTimer.current)
      ACTIVITY_EVENTS.forEach((evt) => window.removeEventListener(evt, resetTimer))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user])

  const value = useMemo(
    () => ({
      user,
      isAuthenticated: !!user,
      isAdmin,
      sessionExpired,
      welcomeMessage,
      login,
      register,
      logout,
      dismissSessionExpired,
      clearWelcomeMessage,
      updateThemePreference,
    }),
    [user, isAdmin, sessionExpired, welcomeMessage, updateThemePreference],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export const useAuth = () => useContext(AuthContext)