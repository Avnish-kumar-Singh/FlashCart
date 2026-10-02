import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { festivalApi } from '../services/api'
import Icon from './Icon'
import { useAuth } from '../context/AuthContext'

function applicationServerKey(value) {
  const padding = '='.repeat((4 - value.length % 4) % 4)
  const base64 = (value + padding).replace(/-/g, '+').replace(/_/g, '/')
  return Uint8Array.from(atob(base64), (character) => character.charCodeAt(0))
}

export default function FestivalBanner() {
  const [festival, setFestival] = useState(null)
  const [pushState, setPushState] = useState('idle')
  const { isAuthenticated } = useAuth()

  useEffect(() => {
    let live = true
    const refresh = () => festivalApi.current()
      .then(({ data }) => { if (live) setFestival(data || null) })
      .catch(() => {})
    refresh()
    const timer = window.setInterval(refresh, 60000)
    return () => { live = false; window.clearInterval(timer) }
  }, [])

  useEffect(() => {
    if (festival?.status === 'ACTIVE') document.documentElement.dataset.festival = 'true'
    else delete document.documentElement.dataset.festival
    return () => { delete document.documentElement.dataset.festival }
  }, [festival?.status])

  if (!festival) return null
  const active = festival.status === 'ACTIVE'
  const name = festival.local_name || festival.name
  const begins = new Date(festival.starts_at).toLocaleDateString('en-IN', { day: 'numeric', month: 'short' })

  const enablePush = async () => {
    if (!isAuthenticated || !('Notification' in window) || !('serviceWorker' in navigator)) {
      setPushState('unsupported')
      return
    }
    setPushState('working')
    try {
      const { data } = await festivalApi.vapidKey()
      if (!data.public_key) {
        setPushState('unconfigured')
        return
      }
      const permission = await Notification.requestPermission()
      if (permission !== 'granted') {
        setPushState('denied')
        return
      }
      const registration = await navigator.serviceWorker.register('/sw.js')
      const subscription = await registration.pushManager.getSubscription() ||
        await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: applicationServerKey(data.public_key) })
      await festivalApi.subscribePush(subscription.toJSON())
      setPushState('enabled')
    } catch {
      setPushState('error')
    }
  }

  return (
    <div className={`festival-banner ${active ? 'is-live' : 'is-upcoming'}`}>
      <span className="festival-mark" aria-hidden="true"><Icon name={active ? 'bolt' : 'clock'} size={15}/></span>
      <span className="festival-message"><b>{active ? `${name} sale is live` : `${name} sale is coming`}</b><small>{active ? `Up to ${festival.discount_percent}% off · ends ${new Date(festival.ends_at).toLocaleTimeString('en-IN', { hour: 'numeric', minute: '2-digit' })}` : `Starts ${begins} · ${festival.discount_percent}% festival offers`}</small></span>
      {isAuthenticated && pushState !== 'enabled' && <button className="festival-alert-button" onClick={enablePush} disabled={pushState === 'working'}>{pushState === 'working' ? 'Enabling…' : 'Enable alerts'}</button>}
      {pushState === 'enabled' && <span className="festival-alert-enabled"><Icon name="check" size={13}/> Alerts on</span>}
      {['denied','unconfigured','unsupported','error'].includes(pushState) && <span className="festival-alert-error">{pushState === 'unconfigured' ? 'Push service unavailable' : pushState === 'denied' ? 'Allow notifications in browser settings' : pushState === 'unsupported' ? 'Browser alerts unavailable' : 'Could not enable alerts'}</span>}
      <Link to="/flash-sale">{active ? 'Shop sale' : 'See details'} <Icon name="arrow" size={14}/></Link>
    </div>
  )
}