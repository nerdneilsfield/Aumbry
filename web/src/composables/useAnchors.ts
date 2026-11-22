import { computed, inject, nextTick, provide, reactive, type InjectionKey } from 'vue'
import { anchorMap, type AnchorInfo } from '@/lib/anchor'
import type { Reading } from '@/composables/useReading'

/**
 * The one locate procedure behind deep links (`#fig-x`), the TOC, <next-steps>, `eq=` chips and `[text](#anchor)` links in
 * <md>. The document's anchor map (id -> target + the stage / collapse / tab / step / section containers on its path) is built
 * once from the parsed XML (lib/anchor.ts); `reveal` then drives the controlled state of those containers centrally, the
 * components only read it:
 *   1. unlock the owning <stages> group up to the target stage (other groups are untouched),
 *   2. open the collapses and switch the tabs / interactive-steps on the path,
 *   3. leave skim mode for the target's section (unfold it),
 *   4. wait until the target is mounted, then scroll to it.
 * (Native hash jumps would fall through: none of that exists in the DOM yet.)
 */
export interface Anchors {
  openCollapses: Set<Element>
  activeTabs: Map<Element, string>
  /** interactive <steps> -> active step (0-based) */
  activeSteps: Map<Element, number>
  /** does the hash point at something in this document? (`#sec-N` and `#stage=N` included) */
  canReveal: (hash: string) => boolean
  /** <next-steps> targets: `^#[a-z0-9-]+$` resolving to an existing element via the prefix table */
  canTarget: (hash: string) => boolean
  /** an element that reuses an id already taken in the shared id namespace */
  isDuplicate: (el: Element) => boolean
  reveal: (hash: string) => Promise<boolean>
  /** same-page navigation: URL + reveal */
  navigate: (hash: string) => Promise<boolean>
}

const KEY: InjectionKey<Anchors> = Symbol('aumbry-anchors')

export const tabKey = (tab: Element) => `tab-${Array.from(tab.parentElement?.children ?? []).indexOf(tab)}`

const frame = () => new Promise<void>((r) => requestAnimationFrame(() => r()))

async function waitFor(id: string, ms = 1200): Promise<HTMLElement | null> {
  const end = performance.now() + ms
  for (;;) {
    const el = document.getElementById(id)
    if (el || performance.now() > end) return el
    await frame()
  }
}

export function provideAnchors(getBody: () => Element | undefined, reading: Reading): Anchors {
  const openCollapses = reactive(new Set<Element>())
  const activeTabs = reactive(new Map<Element, string>())
  const activeSteps = reactive(new Map<Element, number>())

  const map = computed(() => {
    const body = getBody()
    return body ? anchorMap(body) : undefined
  })

  function apply(t: AnchorInfo) {
    // 1. stages: only the group that owns each stage, up to and including that stage
    for (const st of t.stages) {
      const r = reading.stageRef(st)
      if (!r) continue
      if (st === t.el) reading.goTo(r.group, r.index, false)
      else reading.unlockTo(r.group, r.index)
    }
    // 2. collapses, tabs, interactive steps
    for (const c of t.collapses) openCollapses.add(c)
    for (const tab of t.tabs) if (tab.parentElement) activeTabs.set(tab.parentElement, tabKey(tab))
    for (const s of t.steps) activeSteps.set(s.parentElement!, Array.from(s.parentElement!.children).indexOf(s))
    // 3. skim: the section / stage that holds the target (or is the target) shows its own blocks
    for (const u of t.unfold) reading.expand(u)
  }

  const reveal = async (hash: string) => {
    const target = map.value?.find(hash)
    if (!target) return false
    apply(target)
    // 4. mounted, then scroll
    await nextTick()
    const el = await waitFor(target.domId)
    if (!el) return false
    await frame()
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    el.classList.remove('anchor-flash')
    void el.offsetWidth // restart the animation when the same anchor is hit twice
    el.classList.add('anchor-flash')
    return true
  }

  const canReveal = (hash: string) => !!map.value?.find(hash)

  const navigate = async (hash: string) => {
    if (!canReveal(hash)) return false
    try {
      history.pushState(history.state, '', hash)
    } catch {
      /* opaque origin: the jump still works */
    }
    return reveal(hash)
  }

  const anchors: Anchors = {
    openCollapses,
    activeTabs,
    activeSteps,
    canReveal,
    canTarget: (hash) => !!map.value?.findStrict(hash),
    isDuplicate: (el) => !!map.value?.dups.has(el),
    reveal,
    navigate,
  }
  provide(KEY, anchors)
  return anchors
}

export function useAnchors(): Anchors {
  const a = inject(KEY)
  if (!a) throw new Error('useAnchors() outside <XmlRenderer>')
  return a
}
