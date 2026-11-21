<script setup lang="ts">
import { Info, Lightbulb, TriangleAlert } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()

const styles = {
  info: { icon: Info, box: 'border-sky-500/40 bg-sky-500/10', accent: 'text-sky-300' },
  warn: { icon: TriangleAlert, box: 'border-amber-500/40 bg-amber-500/10', accent: 'text-amber-300' },
  insight: { icon: Lightbulb, box: 'border-violet-500/40 bg-violet-500/10', accent: 'text-violet-300' },
}
const type = attr(props.node, 'type')
const s = styles[type as keyof typeof styles] ?? styles.info
</script>

<template>
  <aside :class="['not-prose rounded-xl border p-4', s.box]">
    <div :class="['mb-1.5 flex items-center gap-2 font-semibold', s.accent]">
      <component :is="s.icon" class="size-5 shrink-0" />
      <span v-if="attr(node, 'title')">{{ attr(node, 'title') }}</span>
    </div>
    <MarkdownBlock class="text-sm" :text="node.textContent ?? ''" />
  </aside>
</template>
