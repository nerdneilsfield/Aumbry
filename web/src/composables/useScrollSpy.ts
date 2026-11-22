import { shallowRef, watch, type Ref } from 'vue'
import { useIntersectionObserver } from '@vueuse/core'
import type { TocEntry } from '@/lib/xml'
import { readerBridge } from '@/composables/useReading'

/**
 * Scroll-spy: the active TOC entry is the last (in document order) entry whose section / stage is in the top band of the
 * viewport, so a nested section wins over its parent. The elements are looked up again whenever the renderer (re)mounts or
 * the set of rendered sections changes (stage unlocked, skim toggled, unit unfolded).
 */
export function useScrollSpy(toc: Ref<TocEntry[]>, renderedKey: Ref<unknown>) {
  const active = shallowRef('')
  const sections = shallowRef<HTMLElement[]>([])
  const inBand = new Set<string>()
  watch([toc, renderedKey, () => readerBridge.value?.layoutKey.value], (now, before) => {
    const t = now[0]
    sections.value = t.flatMap((e) => document.getElementById(e.id) ?? [])
    inBand.clear()
    // a new document resets the highlight; a layout change inside the same document keeps it
    if (t !== before?.[0] || !active.value) active.value = t[0]?.id ?? ''
  }, { flush: 'post', immediate: true })
  useIntersectionObserver(sections, (entries) => {
    entries.forEach((e) => (e.isIntersecting ? inBand.add(e.target.id) : inBand.delete(e.target.id)))
    const last = toc.value.filter((t) => inBand.has(t.id)).at(-1)
    if (last) active.value = last.id
  }, { rootMargin: '0px 0px -75% 0px' })
  // Through the renderer when there is one: it unlocks a stage / unfolds a skim unit before scrolling.
  const jump = (id: string) => {
    const r = readerBridge.value
    if (r) void r.reveal(`#${id}`)
    else document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' })
  }
  return { active, jump }
}
