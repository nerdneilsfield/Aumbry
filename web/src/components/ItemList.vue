<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { CalendarDays, Bot } from 'lucide-vue-next'
import type { ItemSummary } from '@/api/types'
import { itemPath } from '@/lib/url'
import { snippetHtml } from '@/lib/sanitize'

type Row = ItemSummary & { snippet?: string }

defineProps<{ items: Row[]; selectable?: boolean }>()
const emit = defineEmits<{ select: [item: Row] }>()
const typeLabel = { report: '报告', post: '精读' }
</script>

<template>
  <ul class="space-y-3">
    <li v-for="item in items" :key="item.id">
      <component
        :is="selectable ? 'button' : RouterLink"
        v-bind="selectable ? { type: 'button' } : { to: itemPath(item) }"
        class="card block w-full cursor-pointer p-4 text-left transition hover:border-gray-500 hover:bg-gray-700/50"
        @click="selectable && emit('select', item)"
      >
        <div class="flex items-start justify-between gap-3">
          <h3 class="font-medium leading-snug text-gray-50">{{ item.title }}</h3>
          <span :class="['chip shrink-0', item.type === 'post' ? 'text-violet-300' : 'text-blue-300']">{{ typeLabel[item.type] }}</span>
        </div>
        <p v-if="item.snippet" class="mt-1.5 text-sm text-gray-300" v-html="snippetHtml(item.snippet)" />
        <p v-else-if="item.summary" class="mt-1.5 line-clamp-2 text-sm text-gray-400">{{ item.summary }}</p>
        <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-gray-400">
          <span class="inline-flex items-center gap-1"><CalendarDays class="size-3.5" />{{ item.date }}</span>
          <span v-if="item.source_agent" class="inline-flex items-center gap-1"><Bot class="size-3.5" />{{ item.source_agent }}</span>
          <span v-for="t in item.tags" :key="t" class="chip">{{ t }}</span>
        </div>
      </component>
    </li>
  </ul>
</template>
