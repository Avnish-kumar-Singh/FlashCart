import { useState } from 'react'
import Icon from './Icon'

// WhatsApp and Facebook both have real web share-intent URLs that work
// without any app SDK or API key. Instagram has no equivalent web share
// URL for arbitrary links, so it falls back to "copy link" with a hint —
// the honest option rather than a broken deep link.
export default function SocialShare({ url, text }) {
  const [copied, setCopied] = useState(false)
  const shareUrl = url || (typeof window !== 'undefined' ? window.location.href : '')
  const shareText = text || 'Check out this flash sale on FlashCart!'

  const whatsappHref = `https://wa.me/?text=${encodeURIComponent(`${shareText} ${shareUrl}`)}`
  const facebookHref = `https://www.facebook.com/sharer/sharer.php?u=${encodeURIComponent(shareUrl)}`

  const copyForInstagram = async () => {
    try {
      await navigator.clipboard.writeText(`${shareText} ${shareUrl}`)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      /* clipboard API unavailable — button simply won't confirm, which is an acceptable degrade */
    }
  }

  return (
    <div className="social-share">
      <a href={whatsappHref} target="_blank" rel="noopener noreferrer" className="share-btn whatsapp" title="Share on WhatsApp">
        <Icon name="share" size={15} /> WhatsApp
      </a>
      <a href={facebookHref} target="_blank" rel="noopener noreferrer" className="share-btn facebook" title="Share on Facebook">
        <Icon name="share" size={15} /> Facebook
      </a>
      <button type="button" className="share-btn instagram" onClick={copyForInstagram} title="Copy link to share on Instagram">
        <Icon name="share" size={15} /> {copied ? 'Link copied!' : 'Instagram'}
      </button>
    </div>
  )
}
