export default function Icon({ name, size = 20 }) {
  const common = { width: size, height: size, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round', strokeLinejoin: 'round' }
  const paths = {
    search: <><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></>,
    user: <><circle cx="12" cy="8" r="3.5"/><path d="M5 20c.7-3.3 3.1-5 7-5s6.3 1.7 7 5"/></>,
    cart: <><path d="M3 4h2l2 11h10l3-8H6"/><circle cx="9" cy="20" r="1"/><circle cx="17" cy="20" r="1"/></>,
    heart: <path d="M20.8 8.8c0 5.3-8.8 10-8.8 10s-8.8-4.7-8.8-10A4.7 4.7 0 0 1 12 6.1a4.7 4.7 0 0 1 8.8 2.7Z"/>,
    arrow: <><path d="M5 12h14"/><path d="m13 6 6 6-6 6"/></>,
    check: <path d="m5 12 4 4L19 6"/>,
    bolt: <path d="m13 2-9 12h7l-1 8 9-12h-7l1-8Z"/>,
    package: <><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z"/><path d="m4.5 7.8 7.5 4 7.5-4M12 12v9"/></>,
    menu: <><path d="M4 7h16M4 12h16M4 17h16"/></>,
    close: <><path d="m6 6 12 12M18 6 6 18"/></>,
    shield: <path d="M12 3 19 6v5c0 4.5-3 8-7 10-4-2-7-5.5-7-10V6l7-3Z"/>,
    bell: <><path d="M6 8a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6Z"/><path d="M10 20a2 2 0 0 0 4 0"/></>,
    clock: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 3"/></>,
    share: <><circle cx="18" cy="5" r="2.5"/><circle cx="6" cy="12" r="2.5"/><circle cx="18" cy="19" r="2.5"/><path d="m8.2 10.8 7.6-4.6M8.2 13.2l7.6 4.6"/></>,
    palette: <><path d="M12 3a9 9 0 1 0 0 18h1.2a2 2 0 0 0 1.4-3.4 1.8 1.8 0 0 1 1.3-3.1H18a3 3 0 0 0 3-3c0-4.7-4-8.5-9-8.5Z"/><circle cx="7.5" cy="11" r=".8" fill="currentColor"/><circle cx="10" cy="7.5" r=".8" fill="currentColor"/><circle cx="14.5" cy="7.5" r=".8" fill="currentColor"/></>,
    sparkles: <><path d="m12 3 1.9 5.8L20 11l-6.1 2.2L12 19l-1.9-5.8L4 11l6.1-2.2L12 3Z"/><path d="m19 14 .8 1.8 2 .7-2 .7L19 19l-.8-1.8-2-.7 2-.7L19 14ZM5 2l.7 1.7 1.8.6-1.8.6L5 6.6l-.7-1.7-1.8-.6 1.8-.6L5 2Z"/></>,
    send: <><path d="m22 2-7 20-4-9-9-4 20-7Z"/><path d="M22 2 11 13"/></>,
    grid: <><rect x="4" y="4" width="6" height="6" rx="1"/><rect x="14" y="4" width="6" height="6" rx="1"/><rect x="4" y="14" width="6" height="6" rx="1"/><rect x="14" y="14" width="6" height="6" rx="1"/></>,
    list: <><path d="M9 6h11M9 12h11M9 18h11"/><path d="M4 6h.01M4 12h.01M4 18h.01"/></>,
    trash: <><path d="M4 7h16"/><path d="M6 7v13a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V7"/><path d="M9 7V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v3"/></>,
  }
  return <svg {...common}>{paths[name] || null}</svg>
}
