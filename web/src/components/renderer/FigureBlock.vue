<script setup lang="ts">
import { computed, onBeforeUnmount, useTemplateRef, watch } from 'vue'
import mediumZoom, { type Zoom } from 'medium-zoom'
import { attr, safeId } from '@/lib/xml'
import { imageSrc } from '@/lib/url'
import { trackRender, usePrinting } from '@/composables/usePrint'

const props = defineProps<{ node: Element }>()
const img = useTemplateRef<HTMLImageElement>('img')
let zoom: Zoom | undefined

// <img> only exists while src is valid, so (re)attach zoom whenever the element appears or goes away.
watch(img, (el) => {
  zoom?.detach()
  zoom = el ? mediumZoom(el, { background: 'rgba(3, 7, 18, 0.92)', margin: 24 }) : undefined
}, { flush: 'post' })
onBeforeUnmount(() => zoom?.detach())

// A picture that was never scrolled to is still unfetched (loading=lazy): printing loads it and waits for it.
const printing = usePrinting()
watch([printing, img], ([p, el]) => {
  if (p && el && !el.complete) trackRender(el.decode().catch(() => undefined))
}, { flush: 'post' })

const src = computed(() => imageSrc(attr(props.node, 'src')))
const caption = computed(() => attr(props.node, 'caption'))
const alt = computed(() => attr(props.node, 'alt') || caption.value)
const anchor = computed(() => (safeId(attr(props.node, 'id')) ? `fig-${attr(props.node, 'id')}` : undefined))
</script>

<template>
  <figure v-if="src" :id="anchor" class="scroll-mt-20 text-center">
    <img
      ref="img"
      :src="src"
      :alt="alt"
      :loading="printing ? 'eager' : 'lazy'"
      class="mx-auto max-h-[32rem] max-w-full cursor-zoom-in rounded-lg border border-gray-700 bg-white dark:bg-gray-950/40"
    />
    <figcaption v-if="caption" class="mt-2 text-sm text-gray-400">{{ caption }}</figcaption>
  </figure>
</template>
