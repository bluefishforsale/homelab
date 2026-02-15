import { useEffect } from 'react'

// Prefetch data on link hover
export function usePrefetch(url: string, enabled: boolean = true) {
  useEffect(() => {
    if (!enabled) return

    // Prefetch on idle
    if ('requestIdleCallback' in window) {
      requestIdleCallback(() => {
        fetch(url, { method: 'HEAD' }).catch(() => {})
      })
    } else {
      setTimeout(() => {
        fetch(url, { method: 'HEAD' }).catch(() => {})
      }, 1)
    }
  }, [url, enabled])
}

// Prefetch on hover
export function prefetchOnHover(url: string) {
  fetch(url).catch(() => {})
}
