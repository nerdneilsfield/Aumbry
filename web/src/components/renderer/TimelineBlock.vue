<script setup lang="ts">
import { computed } from 'vue'
import { attr, children } from '@/lib/xml'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
const events = computed(() => children(props.node).filter((c) => c.tagName === 'event'))
</script>

<template>
  <ol class="not-prose list-none border-l border-gray-700 p-0 pl-6">
    <li v-for="(e, i) in events" :key="i" class="relative pb-6 last:pb-0">
      <span class="absolute -left-[1.9rem] top-1.5 size-3 rounded-full border-2 border-blue-400 bg-gray-900" aria-hidden="true" />
      <time class="font-mono text-xs font-medium tracking-wide text-blue-300">{{ attr(e, 'date') }}</time>
      <h3 class="mt-0.5 text-base font-semibold leading-snug text-gray-50">{{ attr(e, 'title') }}</h3>
      <MarkdownBlock v-if="e.textContent?.trim()" class="md-tight mt-1 text-sm" :text="e.textContent" />
    </li>
  </ol>
</template>
