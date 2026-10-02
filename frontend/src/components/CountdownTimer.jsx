import { useEffect, useState } from 'react'
import Icon from './Icon'

function remainingParts(targetMs) {
  const diff = Math.max(0, targetMs - Date.now())
  const totalSeconds = Math.floor(diff / 1000)
  return {
    h: Math.floor(totalSeconds / 3600),
    m: Math.floor((totalSeconds % 3600) / 60),
    s: totalSeconds % 60,
    done: diff <= 0,
  }
}

function pad(n) { return String(n).padStart(2, '0') }

// Shows a live HH:MM:SS countdown to `targetMs` (epoch millis). Calls
// onEnd once when the timer reaches zero, so parent pages can refresh
// sale status right when it matters instead of only on the next poll.
export default function CountdownTimer({ targetMs, onEnd, label = 'Ends in' }) {
  const [parts, setParts] = useState(() => remainingParts(targetMs))

  useEffect(() => {
    const id = setInterval(() => {
      setParts((prev) => {
        const next = remainingParts(targetMs)
        if (next.done && !prev.done && onEnd) onEnd()
        return next
      })
    }, 1000)
    return () => clearInterval(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetMs])

  if (parts.done) {
    return <span className="countdown-chip ended"><Icon name="clock" size={14} /> Sale ended</span>
  }

  return (
    <span className="countdown-chip">
      <Icon name="clock" size={14} /> {label}{' '}
      {parts.h > 0 && <><b>{pad(parts.h)}</b>:</>}
      <b>{pad(parts.m)}</b>:<b>{pad(parts.s)}</b>
    </span>
  )
}
