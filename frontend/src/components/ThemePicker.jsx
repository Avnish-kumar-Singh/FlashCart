import { useEffect, useState } from 'react'
import { useTheme } from '../context/ThemeContext'
import Icon from './Icon'

export default function ThemePicker({ onClose }) {
  const { theme, preview, themes, previewTheme, cancelPreview, commitTheme } = useTheme()
  const [selected, setSelected] = useState(preview || theme)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    const onKeyDown = (event) => {
      if (event.key === 'Escape') {
        cancelPreview()
        onClose()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [cancelPreview, onClose])

  const close = () => {
    cancelPreview()
    onClose()
  }

  const apply = async () => {
    setSaving(true)
    setError('')
    try {
      await commitTheme(selected)
      onClose()
    } catch (err) {
      setError(err.response?.data?.error || 'Could not save this theme to your profile.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="theme-scrim" onMouseDown={(event) => { if (event.target === event.currentTarget) close() }}>
      <section className="theme-dialog" role="dialog" aria-modal="true" aria-labelledby="theme-title">
        <header className="theme-dialog-head">
          <div><span className="eyebrow">Personalize</span><h2 id="theme-title">Choose your atmosphere</h2><p>Preview a theme, then apply it across FlashCart.</p></div>
          <button className="theme-close" onClick={close} aria-label="Close theme picker"><Icon name="close" size={19}/></button>
        </header>
        <div className="theme-options" role="radiogroup" aria-label="Background theme">
          {themes.map((item) => (
            <button
              type="button"
              role="radio"
              aria-checked={selected === item.id}
              className={`theme-option ${selected === item.id ? 'selected' : ''}`}
              key={item.id}
              onMouseEnter={() => { setSelected(item.id); previewTheme(item.id) }}
              onFocus={() => { setSelected(item.id); previewTheme(item.id) }}
              onClick={() => { setSelected(item.id); previewTheme(item.id) }}
            >
              <span className={`theme-swatch swatch-${item.id}`} aria-hidden="true"><i/><i/><i/></span>
              <span className="theme-option-copy"><b>{item.name}</b><small>{item.detail}</small></span>
              <span className="theme-motion-label">{item.motion}</span>
            </button>
          ))}
        </div>
        {error && <div className="notice error">{error}</div>}
        <footer className="theme-dialog-foot">
          <small>Animations respect your device’s reduced-motion setting.</small>
          <div><button className="ghost-btn" onClick={close}>Cancel</button><button className="primary-btn" disabled={saving} onClick={apply}>{saving ? 'Saving…' : 'Apply theme'}</button></div>
        </footer>
      </section>
    </div>
  )
}