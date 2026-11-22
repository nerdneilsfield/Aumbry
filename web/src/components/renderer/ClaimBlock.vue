<script setup lang="ts">
import { computed } from 'vue'
import { MapPin } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { CLAIM_LEVELS, LEVELS, asLevel } from '@/lib/levels'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
// Unknown level (server rejects it, but be tolerant): fall back to the neutral "claim" look.
const level = computed(() => {
  const l = asLevel(attr(props.node, 'level'))
  return l && CLAIM_LEVELS.includes(l) ? l : 'claim'
})
const s = computed(() => LEVELS[level.value])
const source = computed(() => attr(props.node, 'source'))
</script>

<template>
  <div :class="['not-prose rounded-r-xl border border-l-4 border-gray-700 px-4 py-3', s.bar, s.surface]" :data-level="level">
    <div class="mb-1.5 flex flex-wrap items-center gap-2">
      <span data-export-badge :class="['inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset', s.badge]" :title="s.hint">
        <component :is="s.icon" class="size-3.5" aria-hidden="true" />{{ s.label }}
      </span>
      <span v-if="source" data-export-badge class="inline-flex max-w-full items-center gap-1 rounded-md border border-gray-600 px-1.5 py-0.5 font-mono text-xs text-gray-300">
        <MapPin class="size-3 shrink-0 text-gray-500" aria-hidden="true" /><span class="truncate">{{ source }}</span>
      </span>
    </div>
    <MarkdownBlock class="md-tight text-[0.95rem]" :text="node.textContent ?? ''" />
  </div>
</template>
