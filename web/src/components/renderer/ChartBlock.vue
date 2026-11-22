<script setup lang="ts">
import { computed, defineAsyncComponent, ref, useTemplateRef, watch } from 'vue'
import { useIntersectionObserver } from '@vueuse/core'
import { TriangleAlert } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { fence } from '@/lib/fence'
import { buildChart } from '@/lib/chart'
import { trackRender, usePrinting } from '@/composables/usePrint'
import { useTheme } from '@/composables/useTheme'

const props = defineProps<{ node: Element }>()
const { isDark } = useTheme()

// Lazy chunk: echarts + vue-echarts load only when a <chart> scrolls near the viewport.
const ChartCanvas = defineAsyncComponent({
  loader: () => import('./ChartCanvas.vue'),
  onError: (_e, _retry, fail) => fail(),
})

const json = computed(() => (props.node.textContent ?? '').trim())
const spec = computed(() => buildChart(attr(props.node, 'type'), json.value, isDark.value))
const caption = computed(() => attr(props.node, 'caption'))
// what the Markdown export writes for this figure: the source, not the drawn picture
const exportMd = computed(() => [fence('json', json.value), caption.value && `*${caption.value}*`].filter(Boolean).join('\n\n'))

const root = useTemplateRef<HTMLElement>('root')
const seen = ref(false)
// Printing draws every chart, including those never scrolled to; printDocument() waits until each has finished (ChartCanvas `ready`).
const printing = usePrinting()
const visible = computed(() => seen.value || printing.value)
const { stop } = useIntersectionObserver(root, ([e]) => {
  if (e?.isIntersecting) {
    seen.value = true
    stop()
  }
}, { rootMargin: '600px 0px' })

let ready: (() => void) | undefined
// every (re)draw is tracked: showing the chart, and the theme switch to light that printing causes
watch([visible, spec], ([v, s]) => {
  if (!v || 'error' in s) return
  ready?.()
  trackRender(new Promise<void>((resolve) => (ready = resolve)))
}, { immediate: true })
const onReady = () => ready?.()
</script>

<template>
  <figure ref="root" class="not-prose" :data-export-md="exportMd">
    <div v-if="'error' in spec" class="rounded-xl border border-amber-500/40 bg-amber-500/5">
      <p class="flex items-center gap-2 border-b border-amber-500/30 px-4 py-2 text-sm font-medium text-amber-300">
        <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />图表数据无效，未渲染
      </p>
      <p class="px-4 py-2 text-sm text-gray-300">{{ spec.error }}</p>
      <pre class="max-h-48 overflow-auto border-t border-amber-500/30 px-4 py-3 font-mono text-xs text-gray-400">{{ json }}</pre>
    </div>
    <div v-else class="rounded-xl border border-gray-700 bg-gray-800/60 p-3 sm:p-4">
      <!-- the box keeps its final height from the first paint: no jump when echarts arrives -->
      <div :style="{ height: `${spec.height}px` }" class="print:h-auto!">
        <ChartCanvas v-if="visible" :option="spec.option" :height="spec.height" :label="caption || '图表'" @ready="onReady" />
        <div v-else class="size-full animate-pulse rounded-lg bg-gray-700/30" role="status" aria-label="图表加载中" />
      </div>
    </div>
    <figcaption v-if="caption" class="mt-2 text-center text-sm text-gray-400">{{ caption }}</figcaption>
  </figure>
</template>
