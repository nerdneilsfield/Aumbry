import { computed, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { refDebounced } from '@vueuse/core'
import type { DirSearchParams, ItemType } from '@/api/types'

const first = (v: unknown) => (typeof v === 'string' ? v : Array.isArray(v) && typeof v[0] === 'string' ? v[0] : '')

/**
 * The directory page's search text, tag and type filters. The URL query (`?q=&tag=&type=`) is the single source of
 * truth, so a reload, a shared link and the back button all restore the same view. Typing only touches the URL once
 * the debounce settles, with `replace` so every keystroke does not become a history entry.
 */
export function useDirFilters() {
  const route = useRoute()
  const router = useRouter()

  const urlQ = computed(() => first(route.query.q))
  const tag = computed(() => first(route.query.tag).trim())
  const type = computed<ItemType | ''>(() => {
    const t = first(route.query.type)
    return t === 'report' || t === 'post' ? t : ''
  })

  function setQuery(patch: { q?: string; tag?: string; type?: string }, mode: 'push' | 'replace') {
    const query: LocationQueryRaw = { ...route.query, ...patch }
    for (const key of Object.keys(patch) as (keyof typeof patch)[]) if (!query[key]) delete query[key]
    return router[mode]({ path: route.path, query })
  }

  // The text box shows what the user typed; `dq` lags it and is what reaches the URL and the request.
  const q = ref(urlQ.value)
  const dq = refDebounced(q, 300)
  watch(dq, (v) => {
    if (v.trim() !== urlQ.value.trim()) setQuery({ q: v.trim() }, 'replace')
  })
  // Back/forward or a link changed the URL: show it, unless it is just our own debounced write coming back.
  watch(urlQ, (v) => {
    if (v.trim() !== dq.value.trim()) q.value = v
  })

  /** Any filter in play, judged from the live text box so the list switches as soon as the user types. */
  const active = computed(() => !!(q.value.trim() || tag.value || type.value))
  /** True once the debounced text has caught up with the text box. */
  const settled = computed(() => dq.value.trim() === q.value.trim())
  const params = computed<DirSearchParams>(() => ({ q: dq.value.trim(), type: type.value, tag: tag.value }))

  return {
    q,
    tag,
    type,
    active,
    settled,
    params,
    setType: (t: ItemType | '') => setQuery({ type: t }, 'push'),
    clearTag: () => setQuery({ tag: '' }, 'push'),
  }
}
