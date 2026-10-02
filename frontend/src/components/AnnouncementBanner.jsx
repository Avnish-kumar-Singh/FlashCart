import { useEffect, useState } from 'react'
import { announcementApi } from '../services/api'
import Icon from './Icon'

// Polls for the active admin announcement ("Diwali Sale Begins!" style
// message + festival link) and renders it as a dismissible top banner
// with a bell icon. Dismissal is per-announcement-id and per-tab (sessionStorage)
// so the same banner doesn't reappear on every page nav but does come back
// for a genuinely new announcement.
export default function AnnouncementBanner() {
  const [announcement, setAnnouncement] = useState(null)
  const [dismissed, setDismissed] = useState(false)

  useEffect(() => {
    let cancelled = false
    announcementApi
      .active()
      .then(({ data }) => {
        if (cancelled || !data) return
        setAnnouncement(data)
        setDismissed(sessionStorage.getItem(`flashcart_dismissed_${data.id}`) === '1')
      })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  if (!announcement || dismissed) return null

  const dismiss = () => {
    sessionStorage.setItem(`flashcart_dismissed_${announcement.id}`, '1')
    setDismissed(true)
  }

  const content = (
    <>
      <Icon name="bell" size={16} />
      <span>{announcement.message}</span>
    </>
  )

  return (
    <div className="announcement-banner">
      {announcement.link ? (
        <a href={announcement.link} target="_blank" rel="noopener noreferrer" className="announcement-content">
          {content}
        </a>
      ) : (
        <span className="announcement-content">{content}</span>
      )}
      <button className="announcement-close" onClick={dismiss} aria-label="Dismiss">
        <Icon name="close" size={14} />
      </button>
    </div>
  )
}
