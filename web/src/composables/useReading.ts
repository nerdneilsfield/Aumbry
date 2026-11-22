import { computed, inject, nextTick, onMounted, provide, reactive, shallowRef, watch, type ComputedRef, type InjectionKey, type WritableComputedRef } from 'vue'
import { useEventListener, useStorage } from '@vueuse/core'
import { attr, children, stageDomId } from '@/lib/xml'
import { anchorMap } from '@/lib/anchor'
import { quizCorrect, quizFingerprint, quizKey, quizValid, sameSet } from '@/lib/quiz'
import { stagesIssues } from '@/lib/validate'
import { useExpanded } from '@/composables/usePrint'
import { syncUrlParams, urlParams } from '@/composables/useUrlParams'

/**
 * Reading state of one document. A document can hold several independent <stages> groups (PRD 6.7). What is saved
 * (VueUse useStorage, localStorage; silently in-memory when storage is unavailable, e.g. some file:// setups):
 *   { stages:  Record<stagesId, stageId>              the last stage reached in each group
 *     quizzes: Record<quizId, { fp, picked: number[] }> }  submitted answers: option indexes + a fingerprint
 * `fp` is the SHA-256 (Web Crypto, or @noble/hashes where Web Crypto is unavailable) of question + option order + correct set: when the quiz changes, only its own stored
 * answer is dropped, stage progress stays. A stored value that is not valid JSON (or has the wrong shape) is reset
 * silently. Unknown stage ids fall back to the first stage. Everything else (selection in progress, "expand all",
 * "finished") is per-session. Pacing is soft gating (a UX choice, not access control): locked stages are not rendered.
 */
export interface QuizState {
  picked: string[]
  checked: boolean
}
interface StoredQuiz {
  fp: string
  picked: number[]
}
interface Persisted {
  stages: Record<string, string>
  quizzes: Record<string, StoredQuiz>
}

const emptyState = (): Persisted => ({ stages: {}, quizzes: {} })
const record = (v: unknown): Record<string, unknown> => (v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : {})

/** Reads whatever is in localStorage, keeping only well-formed entries; anything else (including invalid JSON) -> empty. */
export const progressSerializer = {
  read(raw: string): Persisted {
    const out = emptyState()
    try {
      const v = record(JSON.parse(raw))
      for (const [k, stage] of Object.entries(record(v.stages))) if (typeof stage === 'string') out.stages[k] = stage
      for (const [k, q] of Object.entries(record(v.quizzes))) {
        const { fp, picked } = record(q)
        if (typeof fp === 'string' && Array.isArray(picked) && picked.every((n) => Number.isInteger(n) && n >= 0)) out.quizzes[k] = { fp, picked: picked as number[] }
      }
    } catch {
      /* corrupt JSON: start over, the page still loads */
    }
    return out
  },
  write: (v: Persisted) => JSON.stringify(v),
}

export interface Score {
  correct: number
  total: number
  answered: number
}

/** One valid <stages> group (1-8 stages). */
export interface StageGroup {
  el: Element
  id: string
  stages: Element[]
  gated: boolean
}

export interface Reading {
  groups: ComputedRef<StageGroup[]>
  hasStages: ComputedRef<boolean>
  groupOf: (stagesEl: Element) => StageGroup | undefined
  /** the group a <stage> belongs to and its 1-based index there */
  stageRef: (stage: Element) => { group: StageGroup; index: number } | undefined
  /** page-level: every gated group is open (switch or print) */
  allOpen: ComputedRef<boolean>
  expandAll: WritableComputedRef<boolean>
  /** the group shows every stage without "continue" (page-level expand-all, print, or gated="false") */
  isOpen: (g: StageGroup) => boolean
  unlockedCount: (g: StageGroup) => number
  current: (g: StageGroup) => number
  progress: (g: StageGroup) => number
  isUnlocked: (g: StageGroup, i: number) => boolean
  isDone: (g: StageGroup, i: number) => boolean
  isFinished: (g: StageGroup) => boolean
  /** session-only (the saved shape keeps just the last stage reached) */
  finish: (g: StageGroup) => void
  /** unlock this group's stages up to i without moving the viewport (used by anchors); other groups are untouched */
  unlockTo: (g: StageGroup, i: number) => void
  goTo: (g: StageGroup, i: number, scroll?: boolean) => void
  quiz: (id: string) => QuizState
  pick: (id: string, picked: string[]) => void
  check: (id: string) => Promise<void>
  retry: (id: string) => void
  score: (scope?: Element) => Score
  /** the document has at least one <tldr>, so skim mode makes sense */
  hasTldr: ComputedRef<boolean>
  /** skim mode (PRD 9.7): top-level sections and stages show title + tldr only. Always false when there is no tldr. */
  skim: WritableComputedRef<boolean>
  /** is this skim unit (top-level section / stage) currently folded to title + tldr? */
  isCollapsed: (unit: Element) => boolean
  expand: (unit: Element) => void
  collapse: (unit: Element) => void
  /** changes whenever the set of rendered sections changes (stage unlocked, skim toggled, unit expanded) */
  layoutKey: ComputedRef<string>
}

export { stageDomId }

const KEY: InjectionKey<Reading> = Symbol('aumbry-reading')

const rafs = () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r())))
const stageKey = (stage: Element) => stageDomId(stage).slice('stage-'.length)

export function provideReading(getBody: () => Element | undefined, storageKey: string): Reading {
  syncUrlParams()
  const state = useStorage<Persisted>(`aumbry:read:${storageKey}`, emptyState(), undefined, { serializer: progressSerializer, onError: () => {} })
  const expanded = useExpanded()

  // Only well-formed rendered groups take part (unrendered groups, duplicates, or depth > 3 ancestors are excluded).
  const groups = computed<StageGroup[]>(() => {
    const body = getBody()
    if (!body) return []
    const am = anchorMap(body)
    return Array.from(body.querySelectorAll('stages'))
      .filter((el) => !am.isSkipped(el) && stagesIssues(el).length === 0)
      .map((el) => ({ el, id: attr(el, 'id'), stages: children(el).filter((c) => c.tagName === 'stage'), gated: attr(el, 'gated') !== 'false' }))
  })
  const groupOf = (el: Element) => groups.value.find((g) => g.el === el)
  const stageRef = (stage: Element) => {
    const group = groups.value.find((g) => g.el === stage.parentElement)
    return group ? { group, index: group.stages.indexOf(stage) + 1 } : undefined
  }

  // per-session: stage being read, "finished", "expand all"
  const selected = reactive<Record<string, number>>({})
  const finished = reactive(new Set<string>())
  const expandAll = shallowRef(false)

  const total = (g: StageGroup) => g.stages.length
  const indexOf = (g: StageGroup, id: string | undefined) => Math.max(g.stages.findIndex((s) => stageKey(s) === id) + 1, 0)
  const reachedIdx = (g: StageGroup) => indexOf(g, state.value.stages[g.id]) || 1

  const allOpen = computed(() => groups.value.length > 0 && (expandAll.value || expanded.value))
  // a one-stage group has nothing to pace: it is just open
  const isOpen = (g: StageGroup) => allOpen.value || !g.gated || total(g) === 1
  const unlockedCount = (g: StageGroup) => (isOpen(g) ? total(g) : reachedIdx(g))
  const current = (g: StageGroup) => selected[g.id] ?? reachedIdx(g)
  const isUnlocked = (g: StageGroup, i: number) => i >= 1 && i <= unlockedCount(g)
  const isFinished = (g: StageGroup) => finished.has(g.id)
  const isDone = (g: StageGroup, i: number) => i < reachedIdx(g) || (i === total(g) && isFinished(g))
  const progress = (g: StageGroup) => Math.round(((reachedIdx(g) - 1 + (isFinished(g) ? 1 : 0)) / total(g)) * 100)

  const unlockTo = (g: StageGroup, i: number) => {
    const n = Math.min(Math.max(Math.trunc(i) || 1, 1), total(g))
    selected[g.id] = n
    if (n > reachedIdx(g)) state.value.stages[g.id] = stageKey(g.stages[n - 1])
  }

  function goTo(g: StageGroup, i: number, scroll = true) {
    if (i < 1 || i > total(g)) return
    unlockTo(g, i)
    if (!scroll) return
    nextTick().then(rafs).then(() => {
      document.getElementById(stageDomId(g.stages[i - 1]))?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }

  // `#stage=N` deep links (first group): unlock up to N and go there (initial load, hashchange / back-forward).
  // `#stage-<id>` goes through the anchors (XmlRenderer), which unlocks only the group that owns the stage.
  // The address bar is never rewritten by stage navigation: a shared `#stage-<id>` link stays exactly what was shared.
  const fromHash = () => {
    const m = /^#stage=(\d+)$/.exec(location.hash)
    if (m && groups.value[0]) goTo(groups.value[0], Number(m[1]))
  }
  onMounted(fromHash)
  useEventListener(window, 'hashchange', fromHash)

  // ---- quizzes: stored answer = { fp, picked: option indexes }; valid & rendered only (excludes dup id / unrendered ancestors) ----
  const isActiveQuiz = (q: Element) => {
    const body = getBody()
    if (!body || !quizValid(q)) return false
    const am = anchorMap(body)
    return !am.dups.has(q) && !am.isSkipped(q)
  }
  const quizzes = computed(() => new Map(Array.from(getBody()?.querySelectorAll('quiz') ?? []).filter(isActiveQuiz).map((q) => [quizKey(q), q])))
  const fps = reactive(new Map<string, string>())
  const fpReady = shallowRef(false)
  onMounted(async () => {
    try {
      await Promise.all([...quizzes.value].map(async ([k, q]) => fps.set(k, await quizFingerprint(q))))
    } catch {
      // the hash chunk did not load: answers are not restored, but checking an answer computes the fingerprint again
    }
    // a quiz that changed since the answer was stored loses that answer only
    for (const [k, v] of Object.entries(state.value.quizzes)) if (fps.has(k) && fps.get(k) !== v.fp) delete state.value.quizzes[k]
    fpReady.value = true
  })
  const pending = reactive<Record<string, string[]>>({})
  const stored = (id: string) => {
    const v = state.value.quizzes[id]
    return fpReady.value && v && fps.get(id) === v.fp ? v : undefined
  }
  const quiz = (id: string): QuizState => {
    const v = stored(id)
    return v ? { picked: v.picked.map(String), checked: true } : { picked: pending[id] ?? [], checked: false }
  }
  const score = (scope?: Element): Score => {
    const qs = Array.from((scope ?? getBody())?.querySelectorAll('quiz') ?? []).filter(isActiveQuiz)
    const done = qs.filter((q) => quiz(quizKey(q)).checked)
    return { total: qs.length, answered: done.length, correct: done.filter((q) => sameSet(quiz(quizKey(q)).picked, quizCorrect(q))).length }
  }

  // ---- skim mode: `?skim=1|0` in the URL wins, else this item's remembered preference. Independent of stage gating (PRD 9.7). ----
  const skimPref = useStorage(`aumbry:skim:${storageKey}`, false, undefined, { onError: () => {} })
  const hasTldr = computed(() => !!getBody()?.querySelector('tldr'))
  const skimWanted = computed(() => (urlParams.skim === '1' ? true : urlParams.skim === '0' ? false : skimPref.value))
  const skim = computed({
    get: () => hasTldr.value && skimWanted.value,
    set: (v: boolean) => {
      skimPref.value = v
      urlParams.skim = v ? '1' : '0'
    },
  })
  const unfolded = reactive(new Set<Element>())
  watch(skim, () => unfolded.clear())
  // printing / export ignore skim: everything is expanded
  const isCollapsed = (unit: Element) => skim.value && !expanded.value && !unfolded.has(unit)

  const reading: Reading = {
    groups,
    hasStages: computed(() => groups.value.length > 0),
    groupOf,
    stageRef,
    allOpen,
    expandAll: computed({ get: () => expandAll.value, set: (v) => (expandAll.value = v) }),
    isOpen,
    unlockedCount,
    current,
    progress,
    isUnlocked,
    isDone,
    isFinished,
    finish: (g) => void finished.add(g.id),
    unlockTo,
    goTo,
    quiz,
    pick: (id, picked) => {
      if (!stored(id)) pending[id] = picked
    },
    check: async (id) => {
      const q = quizzes.value.get(id)
      if (!q) return
      const fp = fps.get(id) ?? (await quizFingerprint(q))
      state.value.quizzes[id] = { fp, picked: (pending[id] ?? []).map(Number).sort((a, b) => a - b) }
      fps.set(id, fp)
      delete pending[id]
    },
    retry: (id) => {
      delete state.value.quizzes[id]
      delete pending[id]
    },
    score,
    hasTldr,
    skim,
    isCollapsed,
    expand: (u) => void unfolded.add(u),
    collapse: (u) => void unfolded.delete(u),
    layoutKey: computed(() => `${groups.value.map((g) => unlockedCount(g)).join(',')}|${skim.value && !expanded.value}|${unfolded.size}`),
  }
  provide(KEY, reading)
  return reading
}

export function useReading(): Reading {
  const r = inject(KEY)
  if (!r) throw new Error('useReading() outside <XmlRenderer>')
  return r
}

/**
 * The renderer currently on screen, for UI that lives outside it (the TOC and the reading controls in the header): jump to
 * an anchor (unlocking its stage first), ask whether a stage is unlocked, and drive the page-level switches.
 */
export interface ReaderBridge {
  reveal: (hash: string) => Promise<boolean>
  /** is stage `index` (1-based) of the <stages id="group"> group unlocked? */
  isUnlocked: (group: string, index: number) => boolean
  /** 1-based stage the <stages id="group"> group is at (what its step bar shows as active); 0 for a group with nothing to pace */
  currentStage: (group: string) => number
  hasStages: ComputedRef<boolean>
  expandAll: WritableComputedRef<boolean>
  hasTldr: ComputedRef<boolean>
  skim: WritableComputedRef<boolean>
  layoutKey: ComputedRef<string>
}
export const readerBridge = shallowRef<ReaderBridge | null>(null)
