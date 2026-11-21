<script setup lang="ts">
import { computed } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ChevronRight } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'
import XmlNode from './XmlNode.vue'

const props = defineProps<{ node: Element }>()
const { openCollapses } = useAnchors()
// Printing: Reka's forceMount keeps the content mounted (and shown) even while the collapse is closed.
const printing = useMediaQuery('print')

// Collapsed by default; anchors that point inside force it open (see useAnchors).
const open = computed({
  get: () => openCollapses.has(props.node),
  set: (v) => (v ? openCollapses.add(props.node) : openCollapses.delete(props.node)),
})
</script>

<template>
  <CollapsibleRoot v-model:open="open" class="not-prose card overflow-hidden">
    <CollapsibleTrigger class="flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left font-medium text-gray-100 hover:bg-gray-700/40">
      <ChevronRight :class="['size-4 shrink-0 text-gray-400 transition-transform', open && 'rotate-90']" />
      {{ attr(node, 'title') }}
    </CollapsibleTrigger>
    <CollapsibleContent :force-mount="printing" class="space-y-5 border-t border-gray-700 p-4 print:block!">
      <XmlNode v-for="(n, i) in children(node)" :key="i" :node="n" />
    </CollapsibleContent>
  </CollapsibleRoot>
</template>
