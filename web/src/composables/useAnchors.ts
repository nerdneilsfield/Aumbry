import { inject, nextTick, provide, reactive, type InjectionKey } from 'vue'
import { safeId } from '@/lib/xml'

/**
 * In-page anchors (#fig-*, #eq-*, #ref-*, #sec-*) that point into collapsed <collapse> or inactive <tab> content.
 * The targets are located in the XML tree (the DOM does not contain them yet), every collapse / tab ancestor
 * is opened / activated, and only then the page scrolls. Native hash jumps would fall through.
 */
export interface Anchors {
  openCollapses: Set<Element>
  activeTabs: Map<Element, string>
  reveal: (hash: string) => Promise<boolean>
}

const KEY: InjectionKey<Anchors> = Symbol('aumbry-anchors')

export const tabKey = (tab: Element) => `tab-${Array.from(tab.parentElement?.children ?? []).indexOf(tab)}`

const PREFIX: Record<string, string> = { fig: 'figure', eq: 'equation', ref: 'ref' }

const frame = () => new Promise<void>((r) => requestAnimationFrame(() => r()))

async function waitFor(id: string, ms = 800): Promise<HTMLElement | null> {
  const end = performance.now() + ms
  for (;;) {
    const el = document.getElementById(id)
    if (el || performance.now() > end) return el
    await frame()
  }
}

export function provideAnchors(getBody: () => Element | undefined): Anchors {
  const openCollapses = reactive(new Set<Element>())
  const activeTabs = reactive(new Map<Element, string>())

  const locate = (domId: string): Element | undefined => {
    const body = getBody()
    if (!body) return
    const sec = /^sec-(\d+)$/.exec(domId)
    if (sec) return body.ownerDocument.querySelectorAll('section')[Number(sec[1])]
    const m = /^(fig|eq|ref)-(.+)$/.exec(domId)
    if (!m || !safeId(m[2])) return
    return Array.from(body.ownerDocument.querySelectorAll(PREFIX[m[1]])).find((e) => e.getAttribute('id') === m[2])
  }

  const reveal = async (hash: string) => {
    let domId = hash.replace(/^#/, '')
    try {
      domId = decodeURIComponent(domId)
    } catch {
      /* keep raw */
    }
    const target = locate(domId)
    if (!target) return false
    for (let el = target.parentElement; el; el = el.parentElement) {
      if (el.tagName === 'collapse') openCollapses.add(el)
      if (el.tagName === 'tab' && el.parentElement) activeTabs.set(el.parentElement, tabKey(el))
    }
    await nextTick()
    const el = await waitFor(domId)
    if (!el) return false
    await frame()
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    el.classList.remove('anchor-flash')
    void el.offsetWidth // restart the animation when the same anchor is hit twice
    el.classList.add('anchor-flash')
    return true
  }

  const anchors: Anchors = { openCollapses, activeTabs, reveal }
  provide(KEY, anchors)
  return anchors
}

export function useAnchors(): Anchors {
  const a = inject(KEY)
  if (!a) throw new Error('useAnchors() outside <XmlRenderer>')
  return a
}
