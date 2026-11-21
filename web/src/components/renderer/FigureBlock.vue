<script setup lang="ts">
import { computed, onBeforeUnmount, useTemplateRef, watch } from 'vue'
import mediumZoom, { type Zoom } from 'medium-zoom'
import { attr } from '@/lib/xml'
import { imageSrc } from '@/lib/url'

const props = defineProps<{ node: Element }>()
const img = useTemplateRef<HTMLImageElement>('img')
let zoom: Zoom | undefined

// <img> only exists while src is valid, so (re)attach zoom whenever the element appears or goes away.
watch(img, (el) => {
  zoom?.detach()
  zoom = el ? mediumZoom(el, { background: 'rgba(3, 7, 18, 0.92)', margin: 24 }) : undefined
}, { flush: 'post' })
onBeforeUnmount(() => zoom?.detach())

const src = computed(() => imageSrc(attr(props.node, 'src')))
const caption = computed(() => attr(props.node, 'caption'))
</script>

<template>
  <figure v-if="src" class="m-0 text-center">
    <img
      ref="img"
      :src="src"
      :alt="caption"
      loading="lazy"
      class="mx-auto max-h-[32rem] max-w-full cursor-zoom-in rounded-lg border border-gray-700 bg-gray-950/40"
    />
    <figcaption v-if="caption" class="mt-2 text-sm text-gray-400">{{ caption }}</figcaption>
  </figure>
</template>
