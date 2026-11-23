export interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export interface StoredQuiz {
  fp: string
  picked: number[]
}

export interface PersistedReading {
  stages: Record<string, string>
  quizzes: Record<string, StoredQuiz>
}

export const emptyPersistedReading = (): PersistedReading => ({ stages: {}, quizzes: {} })

const asRecord = (v: unknown): Record<string, unknown> =>
  v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {}

/** Reads stored reading progress while filtering malformed entries. */
export const progressSerializer = {
  read(raw: string): PersistedReading {
    const out = emptyPersistedReading()
    try {
      const v = asRecord(JSON.parse(raw))
      for (const [k, stage] of Object.entries(asRecord(v.stages))) {
        if (typeof stage === 'string') out.stages[k] = stage
      }
      for (const [k, q] of Object.entries(asRecord(v.quizzes))) {
        const { fp, picked } = asRecord(q)
        if (typeof fp === 'string' && Array.isArray(picked) && picked.every((n) => Number.isInteger(n) && n >= 0)) {
          out.quizzes[k] = { fp, picked: picked as number[] }
        }
      }
    } catch {
      /* corrupt JSON: fall back to empty state */
    }
    return out
  },
  write: (v: PersistedReading) => JSON.stringify(v),
}

/**
 * Storage adapter passed directly to VueUse `useStorage` for both `aumbry:read:<id>` and `aumbry:skim:<id>`.
 * When `readOnly` is true (e.g. `?rev=N` historical snapshot view), returns an isolated in-memory Map-backed
 * storage that never reads, writes, or removes any key on `backing` (`localStorage`).
 */
export function createReadingStorage(readOnly: boolean, backing?: StorageLike | null): StorageLike {
  const mem = new Map<string, string>()
  return {
    getItem(key) {
      if (readOnly || !backing) return mem.get(key) ?? null
      return backing.getItem(key)
    },
    setItem(key, value) {
      if (readOnly || !backing) {
        mem.set(key, value)
        return
      }
      backing.setItem(key, value)
    },
    removeItem(key) {
      if (readOnly || !backing) {
        mem.delete(key)
        return
      }
      backing.removeItem(key)
    },
  }
}

/** Reconciles a reactive query-parameter record with a raw query string (e.g. preserving `rev` and `dir`). */
export function syncQueryRecord(searchStr: string, record: Record<string, string | string[] | undefined>): void {
  const search = new URLSearchParams(searchStr)
  const unused = new Set(Object.keys(record))
  for (const key of search.keys()) {
    const all = search.getAll(key)
    const next = all.length > 1 ? all : (search.get(key) ?? '')
    const cur = record[key]
    const same = Array.isArray(next)
      ? Array.isArray(cur) && cur.length === next.length && next.every((v, i) => cur[i] === v)
      : cur === next
    if (!same) record[key] = next
    unused.delete(key)
  }
  for (const key of unused) delete record[key]
}

/** Removes one query parameter from `searchStr` while keeping all other parameters (including `rev` and `dir`). */
export function stripQueryParam(searchStr: string, key: string): string {
  const search = new URLSearchParams(searchStr)
  search.delete(key)
  const out = search.toString()
  return out ? `?${out}` : ''
}
