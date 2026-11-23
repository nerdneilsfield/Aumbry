import type { QueryClient } from '@tanstack/vue-query'

const AUTH_PREFIXES = new Set([
  'admin',
  'item',
  'item-content',
  'item-revisions',
  'item-rev-content',
  'dir',
  'dir-search',
])

/** Cancels and evicts all session-dependent query caches on logout, expiry, or login change. */
export function clearAuthSensitiveQueries(qc: QueryClient): void {
  const predicate = (q: { queryKey: readonly unknown[] }) => {
    const head = q.queryKey[0]
    if (typeof head !== 'string' || !AUTH_PREFIXES.has(head)) return false
    return !(head === 'admin' && (q.queryKey[1] === 'me' || q.queryKey[1] === 'login'))
  }
  void qc.cancelQueries({ predicate })
  qc.removeQueries({ predicate })
}
