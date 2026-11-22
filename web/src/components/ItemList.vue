<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { CalendarDays, Bot } from 'lucide-vue-next'
import type { ItemSummary } from '@/api/types'
import { itemPath } from '@/lib/url'
import { snippetHtml } from '@/lib/sanitize'
import TagChip from '@/components/TagChip.vue'

type Row = ItemSummary & { snippet?: string }

/**
 * `selectable` turns each row into a button (admin). `dirId` is the directory the list belongs to: rows then open
 * with `?dir=` (so the reader knows which directory to take previous/next from) and their tags filter that directory.
 */
defineProps<{ items: Row[]; selectable?: boolean; dirId?: string; activeTag?: string }>()
const emit = defineEmits<{ select: [item: Row] }>()
const typeLabel = { report: '报告', post: '精读' }
</script>

<template>
  <ul class="space-y-3">
    <!-- The title is a stretched link/button covering the whole card; tag chips sit above it so they stay separate controls. -->
    <li v-for="item in items" :key="item.id" class="card relative p-4 transition focus-within:border-gray-500 hover:border-gray-500 hover:bg-gray-700/50">
      <div class="flex items-start justify-between gap-3">
        <h3 class="font-medium leading-snug text-gray-50">
          <component
            :is="selectable ? 'button' : RouterLink"
            v-bind="selectable ? { type: 'button' } : { to: { path: itemPath(item), query: dirId ? { dir: dirId } : undefined } }"
            class="cursor-pointer text-left outline-none after:absolute after:inset-0 after:rounded-xl focus-visible:after:ring-2 focus-visible:after:ring-blue-400"
            @click="selectable && emit('select', item)"
          >{{ item.title }}</component>
        </h3>
        <span :class="['chip shrink-0', item.type === 'post' ? 'text-violet-300' : 'text-blue-300']">{{ typeLabel[item.type] }}</span>
      </div>
      <p v-if="item.snippet" class="mt-1.5 text-sm text-gray-300" v-html="snippetHtml(item.snippet)" />
      <p v-else-if="item.summary" class="mt-1.5 line-clamp-2 text-sm text-gray-400">{{ item.summary }}</p>
      <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-gray-400">
        <span class="inline-flex items-center gap-1"><CalendarDays class="size-3.5" />{{ item.date }}</span>
        <span v-if="item.source_agent" class="inline-flex items-center gap-1"><Bot class="size-3.5" />{{ item.source_agent }}</span>
        <TagChip v-for="t in item.tags" :key="t" :tag="t" :dir-id="dirId" :active="t === activeTag" class="relative" />
      </div>
    </li>
  </ul>
</template>
