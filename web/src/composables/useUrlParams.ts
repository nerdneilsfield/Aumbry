import { useUrlSearchParams } from '@vueuse/core'

/**
 * The page's query string as one reactive object (VueUse). A single shared instance: two instances would each write
 * their own stale copy of the query back (e.g. the offline demo's `?doc=` and the reader's `?skim=`).
 */
export const urlParams = useUrlSearchParams<{ doc?: string; skim?: string }>('history')
