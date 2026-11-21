<script setup lang="ts">
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ChevronRight } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import XmlNode from './XmlNode.vue'

defineProps<{ node: Element }>()
</script>

<template>
  <!-- collapsed by default -->
  <CollapsibleRoot v-slot="{ open }" class="not-prose card overflow-hidden">
    <CollapsibleTrigger class="flex w-full cursor-pointer items-center gap-2 px-4 py-3 text-left font-medium text-gray-100 hover:bg-gray-700/40">
      <ChevronRight :class="['size-4 shrink-0 text-gray-400 transition-transform', open && 'rotate-90']" />
      {{ attr(node, 'title') }}
    </CollapsibleTrigger>
    <CollapsibleContent class="space-y-5 border-t border-gray-700 p-4">
      <XmlNode v-for="(n, i) in children(node)" :key="i" :node="n" />
    </CollapsibleContent>
  </CollapsibleRoot>
</template>
