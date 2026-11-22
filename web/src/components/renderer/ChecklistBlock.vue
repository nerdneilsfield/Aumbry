<script setup lang="ts">
import { computed } from 'vue'
import { Circle, CircleCheck, OctagonX, TriangleAlert } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { mdLine } from '@/lib/mdText'

const props = defineProps<{ node: Element }>()

const STATUS = {
  done: { label: '完成', icon: CircleCheck, icon_cls: 'text-emerald-400', badge: 'bg-emerald-500/15 text-emerald-300 ring-emerald-400/40' },
  todo: { label: '待办', icon: Circle, icon_cls: 'text-gray-500', badge: 'bg-gray-500/20 text-gray-300 ring-gray-500/40' },
  blocked: { label: '阻塞', icon: OctagonX, icon_cls: 'text-red-400', badge: 'bg-red-500/15 text-red-300 ring-red-400/40' },
  risk: { label: '风险', icon: TriangleAlert, icon_cls: 'text-amber-400', badge: 'bg-amber-500/15 text-amber-300 ring-amber-400/40' },
}
const items = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'check')
    .map((c) => ({
      status: STATUS[attr(c, 'status') as keyof typeof STATUS] ?? STATUS.todo,
      text: c.textContent?.trim() ?? '',
      detail: attr(c, 'detail'),
    })),
)

// what the Markdown export writes: a task list, done ticked; blocked / risk stay unticked and keep their label; detail goes under the item
const exportMd = computed(() =>
  items.value.length === 0
    ? undefined
    : items.value
        .map((it) => {
          const label = it.status.label === '阻塞' || it.status.label === '风险' ? `[${it.status.label}] ` : ''
          return [`- [${it.status.label === '完成' ? 'x' : ' '}] ${label}${mdLine(it.text)}`, it.detail && `  > ${mdLine(it.detail)}`].filter(Boolean).join('\n')
        })
        .join('\n'),
)
</script>

<template>
  <ul class="not-prose list-none divide-y divide-gray-700/70 overflow-hidden rounded-xl border border-gray-700 bg-gray-800/50 p-0" :data-export-md="exportMd">
    <li v-for="(it, i) in items" :key="i" class="flex items-start gap-3 px-4 py-3">
      <component :is="it.status.icon" :class="['mt-0.5 size-5 shrink-0', it.status.icon_cls]" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <p :class="['text-[0.95rem] text-gray-100', it.status.label === '完成' && 'text-gray-300']">{{ it.text }}</p>
        <p v-if="it.detail" class="mt-0.5 text-sm text-gray-400">{{ it.detail }}</p>
      </div>
      <span :class="['shrink-0 rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset', it.status.badge]">{{ it.status.label }}</span>
    </li>
  </ul>
</template>
