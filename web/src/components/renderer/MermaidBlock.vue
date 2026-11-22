<script setup lang="ts">
import { computed, ref, shallowRef, useTemplateRef, watch } from 'vue'
import { useIntersectionObserver } from '@vueuse/core'
import { TriangleAlert } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { fence } from '@/lib/fence'
import { renderMermaid } from '@/lib/mermaid'
import { trackRender, usePrinting } from '@/composables/usePrint'
import { useTheme } from '@/composables/useTheme'

const props = defineProps<{ node: Element }>()
const { isDark } = useTheme()
const source = computed(() => (props.node.textContent ?? '').trim())
const caption = computed(() => attr(props.node, 'caption'))
// what the Markdown export writes for this figure: the source, not the drawn picture
const exportMd = computed(() => [fence('mermaid', source.value), caption.value && `*${caption.value}*`].filter(Boolean).join('\n\n'))

const root = useTemplateRef<HTMLElement>('root')
const seen = ref(false)
// Printing draws every diagram, including those never scrolled to, and printDocument() waits for them (trackRender).
const printing = usePrinting()
const visible = computed(() => seen.value || printing.value)
const svg = ref('')
const error = shallowRef<string>()
const busy = ref(true)

// Start loading mermaid only when the block is about to scroll into view.
const { stop } = useIntersectionObserver(root, ([e]) => {
  if (e?.isIntersecting) {
    seen.value = true
    stop()
  }
}, { rootMargin: '600px 0px' })

let run = 0
watch([visible, source, isDark], async ([v, src, dark]) => {
  if (!v) return
  const me = ++run
  busy.value = true
  try {
    const out = await trackRender(renderMermaid(src, dark))
    if (me !== run) return
    svg.value = out
    error.value = undefined
  } catch (e) {
    if (me !== run) return
    svg.value = ''
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    if (me === run) busy.value = false
  }
}, { immediate: true })
</script>

<template>
  <figure ref="root" class="not-prose" :data-export-md="exportMd">
    <!-- error: keep the page intact, show the source and why it failed -->
    <div v-if="error" class="rounded-xl border border-amber-500/40 bg-amber-500/5">
      <p class="flex items-center gap-2 border-b border-amber-500/30 px-4 py-2 text-sm font-medium text-amber-300">
        <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />图表渲染失败，以下为 Mermaid 源码
      </p>
      <pre class="overflow-x-auto px-4 py-3 font-mono text-xs leading-relaxed text-gray-300">{{ source }}</pre>
      <p class="whitespace-pre-wrap break-words border-t border-amber-500/30 px-4 py-2 font-mono text-xs text-gray-400">{{ error.split('\n').slice(0, 6).join('\n') }}</p>
    </div>
    <!-- reserved height + skeleton while mermaid is fetched and drawing: no layout jump -->
    <div v-else-if="!svg" class="flex min-h-64 animate-pulse items-center justify-center rounded-xl border border-gray-700 bg-gray-800/60 text-sm text-gray-500" role="status">
      <span v-if="busy">图表加载中…</span>
    </div>
    <!-- eslint-disable-next-line vue/no-v-html -- mermaid strict output re-sanitised with DOMPurify's SVG profile -->
    <div v-else class="overflow-x-auto rounded-xl border border-gray-700 bg-white p-4 dark:bg-gray-950/40 [&_svg]:mx-auto [&_svg]:h-auto max-sm:[&_svg]:min-w-[30rem]" role="img" :aria-label="caption || 'Mermaid 图'" v-html="svg" />
    <figcaption v-if="caption" class="mt-2 text-center text-sm text-gray-400">{{ caption }}</figcaption>
  </figure>
</template>
