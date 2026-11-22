import { useUrlSearchParams } from '@vueuse/core'

/**
 * The page's query string as one reactive object (VueUse). A single shared instance: two instances would each write
 * their own stale copy of the query back (e.g. the offline demo's `?doc=` and the reader's `?skim=`).
 */
export const urlParams = useUrlSearchParams<{ doc?: string; skim?: string }>('history')

/** Re-reads window.location.search after pushState route navigation (VueUse only listens to popstate). */
export function syncUrlParams() {
  if (typeof window === 'undefined') return
  const search = new URLSearchParams(window.location.search)
  const record = urlParams as Record<string, string | string[] | undefined>
  const unused = new Set(Object.keys(record))
  for (const key of search.keys()) {
    const all = search.getAll(key)
    const next = all.length > 1 ? all : (search.get(key) ?? '')
    const cur = record[key]
    const same = Array.isArray(next) ? Array.isArray(cur) && cur.length === next.length && next.every((v, i) => cur[i] === v) : cur === next
    if (!same) record[key] = next
    unused.delete(key)
  }
  for (const key of unused) delete record[key]
}

/** Synchronously removes `?skim` from both window.location and urlParams when switching documents. */
export function clearSkimParam() {
  if (typeof window !== 'undefined') {
    const u = new URL(window.location.href)
    if (u.searchParams.has('skim')) {
      u.searchParams.delete('skim')
      const search = u.searchParams.toString()
      try {
        history.replaceState(history.state, '', u.pathname + (search ? `?${search}` : '') + u.hash)
      } catch {
        /* opaque origins may refuse */
      }
    }
    syncUrlParams()
  }
  if ('skim' in urlParams) delete urlParams.skim
}
