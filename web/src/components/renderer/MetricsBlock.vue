<script setup lang="ts">
import { computed } from 'vue'
import { ArrowDownRight, ArrowUpRight, Minus } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { mdCell } from '@/lib/mdText'

const props = defineProps<{ node: Element }>()

const trends = {
  up: { icon: ArrowUpRight, cls: 'text-emerald-300', label: '上升' },
  down: { icon: ArrowDownRight, cls: 'text-rose-300', label: '下降' },
  flat: { icon: Minus, cls: 'text-gray-400', label: '持平' },
}
const items = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'metric')
    .slice(0, 12)
    .map((m) => ({
      label: attr(m, 'label'),
      value: attr(m, 'value'),
      unit: attr(m, 'unit'),
      delta: attr(m, 'delta'),
      trend: trends[attr(m, 'trend') as keyof typeof trends],
    })),
)

// what the Markdown export writes: the same fields the cards show, as a GFM table
const exportMd = computed(() =>
  items.value.length === 0
    ? undefined
    : [
        '| 指标 | 值 | 变化 |',
        '| --- | --- | --- |',
        ...items.value.map((m) => {
          const change = [m.delta && m.trend?.label, m.delta].filter(Boolean).join(' ')
          return `| ${mdCell(m.label)} | ${mdCell([m.value, m.unit].filter(Boolean).join(' '))} | ${mdCell(change)} |`
        }),
      ].join('\n'),
)
</script>

<template>
  <dl class="not-prose grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-4" :data-export-md="exportMd">
    <div v-for="(m, i) in items" :key="i" class="card flex min-w-0 flex-col gap-1 px-4 py-3.5">
      <dt class="truncate text-xs font-medium text-gray-400" :title="m.label">{{ m.label }}</dt>
      <dd class="m-0 flex items-baseline gap-1 text-gray-50">
        <span class="truncate text-2xl font-bold tabular-nums leading-tight">{{ m.value }}</span>
        <span v-if="m.unit" class="shrink-0 text-sm text-gray-400">{{ m.unit }}</span>
      </dd>
      <dd v-if="m.delta" :class="['m-0 inline-flex items-center gap-0.5 text-xs font-medium tabular-nums', m.trend?.cls ?? 'text-gray-400']">
        <component :is="m.trend.icon" v-if="m.trend" class="size-3.5" aria-hidden="true" />
        <span v-if="m.trend" class="sr-only">{{ m.trend.label }}</span>{{ m.delta }}
      </dd>
    </div>
  </dl>
</template>
