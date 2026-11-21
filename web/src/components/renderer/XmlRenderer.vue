<script setup lang="ts">
import { watch } from 'vue'
import { useRoute } from 'vue-router'
import { children } from '@/lib/xml'
import { provideAnchors } from '@/composables/useAnchors'
import XmlNode from './XmlNode.vue'

const props = defineProps<{ body: Element }>()
const route = useRoute()
const { reveal } = provideAnchors(() => props.body)

// Direct loads / back-forward with #fig-x, #eq-x, #ref-x: open whatever hides the target, then scroll.
watch(() => route.hash, (h) => h && void reveal(h), { immediate: true, flush: 'post' })

// Same-page links inside the document (markdown [Fig. 4](#fig-arch) etc.) go through reveal() as well.
function onClick(e: MouseEvent) {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  const a = (e.target as Element | null)?.closest('a[href^="#"]')
  const href = a?.getAttribute('href')
  if (!a || !href || href === '#') return
  e.preventDefault()
  history.pushState(history.state, '', href)
  void reveal(href)
}
</script>

<template>
  <div class="space-y-10" @click="onClick">
    <XmlNode v-for="(n, i) in children(body)" :key="i" :node="n" />
  </div>
</template>
