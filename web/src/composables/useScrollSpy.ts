import { shallowRef, watch, type Ref } from 'vue'
import { useIntersectionObserver } from '@vueuse/core'
import type { TocEntry } from '@/lib/xml'

/**
 * Scroll-spy: the active TOC entry is the last section whose heading crossed the top of the viewport.
 * `renderedKey` changes whenever the renderer (re)mounts so the section elements are looked up again.
 */
export function useScrollSpy(toc: Ref<TocEntry[]>, renderedKey: Ref<unknown>) {
  const active = shallowRef('')
  const sections = shallowRef<HTMLElement[]>([])
  watch([toc, renderedKey], ([t]) => {
    sections.value = t.flatMap((e) => document.getElementById(e.id) ?? [])
    active.value = t[0]?.id ?? ''
  }, { flush: 'post', immediate: true })
  useIntersectionObserver(sections, (entries) => entries.forEach((e) => e.isIntersecting && (active.value = e.target.id)), {
    rootMargin: '0px 0px -75% 0px',
  })
  const jump = (id: string) => document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' })
  return { active, jump }
}
