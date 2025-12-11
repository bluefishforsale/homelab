// Simple service worker for offline caching of API responses
const CACHE_NAME = 'ai-corp-v1'

// Cache-first strategy for API calls
self.addEventListener('fetch', (event: any) => {
  const url = new URL(event.request.url)
  
  // Only cache API GET requests
  if (event.request.method !== 'GET' || !url.pathname.startsWith('/api/')) {
    return
  }

  event.respondWith(
    caches.open(CACHE_NAME).then(cache => {
      return cache.match(event.request).then(cachedResponse => {
        const fetchPromise = fetch(event.request).then(networkResponse => {
          // Update cache in background
          if (networkResponse.ok) {
            cache.put(event.request, networkResponse.clone())
          }
          return networkResponse
        }).catch(() => cachedResponse) // Fallback to cache on network error

        // Return cached response immediately if available
        return cachedResponse || fetchPromise
      })
    })
  )
})

// Clean old caches
self.addEventListener('activate', (event: any) => {
  event.waitUntil(
    caches.keys().then(cacheNames => {
      return Promise.all(
        cacheNames
          .filter(name => name !== CACHE_NAME)
          .map(name => caches.delete(name))
      )
    })
  )
})

export {}
