<script setup lang="ts">
import { computed } from 'vue'
import { attr, children } from '@/lib/xml'
import XmlNode from './XmlNode.vue'

const props = defineProps<{ node: Element }>()
const steps = computed(() => children(props.node).filter((c) => c.tagName === 'step'))
</script>

<template>
  <ol class="not-prose list-none p-0">
    <li v-for="(s, i) in steps" :key="i" class="relative flex gap-4 pb-6 last:pb-0">
      <!-- rail -->
      <span v-if="i < steps.length - 1" class="absolute bottom-0 left-4 top-9 w-px -translate-x-1/2 bg-gray-700" aria-hidden="true" />
      <span class="relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full border border-blue-400/60 bg-blue-500/15 text-sm font-bold text-blue-300 tabular-nums">{{ i + 1 }}</span>
      <div class="min-w-0 flex-1 space-y-3 pt-0.5">
        <h3 class="text-base font-semibold leading-8 text-gray-50">{{ attr(s, 'title') }}</h3>
        <XmlNode v-for="(n, j) in children(s)" :key="j" :node="n" />
      </div>
    </li>
  </ol>
</template>
