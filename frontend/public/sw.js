self.addEventListener('push', (event) => {
  let message = {}
  try {
    message = event.data ? event.data.json() : {}
  } catch {
    message = { body: event.data?.text() || '' }
  }
  event.waitUntil(self.registration.showNotification(message.title || 'FlashCart festival sale', {
    body: message.body || 'A festival sale is live now.',
    tag: message.tag || 'flashcart-festival',
    data: { url: message.url || '/flash-sale' },
    renotify: false,
  }))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = event.notification.data?.url || '/flash-sale'
  event.waitUntil(clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windows) => {
    const existing = windows.find((client) => new URL(client.url).origin === self.location.origin)
    if (existing) return existing.navigate(target).then(() => existing.focus())
    return clients.openWindow(target)
  }))
})