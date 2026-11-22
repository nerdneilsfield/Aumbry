<script setup lang="ts">
import { computed } from 'vue'
import { attr, children } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'
import FigureBlock from './FigureBlock.vue'
import DuplicateId from './DuplicateId.vue'

const props = defineProps<{ node: Element }>()
const { isDuplicate } = useAnchors()
const figures = computed(() => children(props.node).filter((c) => c.tagName === 'figure').slice(0, 6))
const cols = computed(() => (attr(props.node, 'cols') === '3' ? 3 : 2))
</script>

<template>
  <figure class="not-prose">
    <div :class="['grid items-start gap-4', cols === 3 ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3' : 'grid-cols-1 sm:grid-cols-2']">
      <div v-for="(f, i) in figures" :key="i" class="min-w-0">
        <p class="mb-1 text-left font-mono text-xs font-semibold text-gray-400">({{ String.fromCharCode(97 + i) }})</p>
        <DuplicateId v-if="isDuplicate(f)" :node="f" />
        <FigureBlock v-else :node="f" />
      </div>
    </div>
    <figcaption v-if="attr(node, 'caption')" class="mt-3 text-center text-sm text-gray-400">{{ attr(node, 'caption') }}</figcaption>
  </figure>
</template>
