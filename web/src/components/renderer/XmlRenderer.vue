<script setup lang="ts">
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { provideAnchors } from '@/composables/useAnchors'
import { provideReading, readerBridge, type ReaderBridge } from '@/composables/useReading'
import ContainerBody from './ContainerBody.vue'

// storageKey: identifies the document for persisted reading state (unlocked stages, quiz answers).
// readOnly: isolates `?rev=N` historical views from reading or writing localStorage.
const props = withDefaults(defineProps<{ body: Element; storageKey?: string; readOnly?: boolean }>(), { storageKey: 'doc', readOnly: false })
const route = useRoute()
const reading = provideReading(() => props.body, props.storageKey, { readOnly: props.readOnly })
const { reveal, navigate, canReveal } = provideAnchors(() => props.body, reading)

// Direct loads / back-forward with #fig-x, #eq-x, #ref-x, #stage-x: open whatever hides the target, then scroll.
// (location.hash too: the offline demo runs the router on memory history.)
watch(() => route.hash || location.hash, (h) => h && void reveal(h), { immediate: true, flush: 'post' })

// Same-page Aumbry anchor links inside the document (markdown [Fig. 4](#fig-arch) etc.) go through navigate();
// local markdown footnote anchors (#fn-md1-1 / #fnref-md1-1) keep native browser scrolling.
function onClick(e: MouseEvent) {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  const a = (e.target as Element | null)?.closest('a[href^="#"]')
  const href = a?.getAttribute('href')
  if (!a || !href || href === '#' || !canReveal(href)) return
  e.preventDefault()
  void navigate(href)
}

// Lets UI outside the renderer (the TOC) unlock stages and jump.
const bridge: ReaderBridge = {
  reveal,
  isUnlocked: (group, index) => {
    const g = reading.groups.value.find((x) => x.id === group)
    return !!g && reading.isUnlocked(g, index)
  },
  currentStage: (group) => {
    const g = reading.groups.value.find((x) => x.id === group)
    return g && g.stages.length > 1 ? reading.current(g) : 0
  },
  hasStages: reading.hasStages,
  expandAll: reading.expandAll,
  hasTldr: reading.hasTldr,
  skim: reading.skim,
  layoutKey: reading.layoutKey,
}
onMounted(() => (readerBridge.value = bridge))
onBeforeUnmount(() => {
  if (readerBridge.value === bridge) readerBridge.value = null
})
</script>

<template>
  <div class="space-y-10" @click="onClick">
    <!-- blocks placed directly in <body> are the "正文": in skim mode they fold into one block with its own button -->
    <ContainerBody :node="body" fold-label="展开正文" />
  </div>
</template>
