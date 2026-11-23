import { computed } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { api } from '@/api/client'
import { clearAuthSensitiveQueries } from '@/lib/queryCache'

/** Admin session + the shared directory tree; the state itself lives in the query cache, so every caller sees the same data. */
export function useAdmin() {
  const qc = useQueryClient()

  const me = useQuery({ queryKey: ['admin', 'me'], queryFn: api.admin.me })
  const session = computed(() => (me.isPending.value ? 'unknown' : me.data.value?.admin ? 'in' : 'out'))

  const treeQuery = useQuery({
    queryKey: ['admin', 'tree'],
    queryFn: api.admin.tree,
    enabled: () => session.value === 'in',
  })
  const tree = computed(() => treeQuery.data.value?.tree ?? [])
  const unmountedCount = computed(() => treeQuery.data.value?.unmounted_count ?? 0)

  const login = useMutation({
    mutationKey: ['admin', 'login'],
    mutationFn: (v: { username: string; password: string }) => api.admin.login(v.username, v.password),
    onSuccess: () => {
      clearAuthSensitiveQueries(qc)
      return qc.invalidateQueries({ queryKey: ['admin', 'me'] })
    },
  })

  const logout = useMutation({
    mutationKey: ['admin', 'logout'],
    mutationFn: api.admin.logout,
    onSettled: () => {
      clearAuthSensitiveQueries(qc)
      qc.setQueryData(['admin', 'me'], { admin: false })
    },
  })

  /** Call after any admin write: directory tree and item lists are re-fetched. */
  const invalidate = () => qc.invalidateQueries({ predicate: (q) => q.queryKey[0] === 'admin' && q.queryKey[1] !== 'me' })

  return { session, tree, unmountedCount, login, logout, invalidate }
}
