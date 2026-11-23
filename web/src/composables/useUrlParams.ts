import { useUrlSearchParams } from '@vueuse/core'
import { stripQueryParam, syncQueryRecord } from '@/lib/readingState'

/**
 * The page's query string as one reactive object (VueUse). A single shared instance: two instances would each write
 * their own stale copy of the query back (e.g. `?doc=`, `?skim=`, `?dir=`, `?rev=`).
 */
export const urlParams = useUrlSearchParams<{ doc?: string; skim?: string; dir?: string; rev?: string }>('history')

/** Re-reads window.location.search after pushState route navigation (VueUse only listens to popstate). */
export function syncUrlParams() {
  if (typeof window === 'undefined') return
  syncQueryRecord(window.location.search, urlParams as Record<string, string | string[] | undefined>)
}

/** Synchronously removes `?skim` from both window.location and urlParams when switching documents without dropping `rev`/`dir`. */
export function clearSkimParam() {
  if (typeof window !== 'undefined') {
    const u = new URL(window.location.href)
    if (u.searchParams.has('skim')) {
      const nextSearch = stripQueryParam(u.search, 'skim')
      try {
        history.replaceState(history.state, '', u.pathname + nextSearch + u.hash)
      } catch {
        /* opaque origins may refuse */
      }
    }
    syncUrlParams()
  }
  if ('skim' in urlParams) delete urlParams.skim
}
