<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import type { ItemSummary, Neighbors } from '@/api/types'
import { itemPath } from '@/lib/url'

const props = defineProps<{ neighbors: Neighbors }>()

// `?dir=` keeps the same directory selected while paging, even when the item is mounted in several.
const to = (item: ItemSummary) => ({ path: itemPath(item), query: { dir: props.neighbors.dir.id } })
const link = 'card group flex min-w-0 flex-col gap-1 p-4 outline-none transition hover:border-gray-500 hover:bg-gray-700/50 focus-visible:ring-2 focus-visible:ring-blue-400'
</script>

<template>
  <nav v-if="neighbors.prev || neighbors.next" :aria-label="`「${neighbors.dir.name}」中的上一篇与下一篇`" class="mt-14 border-t border-gray-700 pt-8 print:hidden">
    <p class="mb-3 text-xs text-gray-500">按日期，目录「{{ neighbors.dir.name }}」</p>
    <div class="grid gap-3 sm:grid-cols-2">
      <RouterLink v-if="neighbors.prev" :to="to(neighbors.prev)" rel="prev" :class="link" data-neighbor="prev">
        <span class="inline-flex items-center gap-1 text-xs text-gray-400"><ChevronLeft class="size-4" />上一篇</span>
        <span class="truncate font-medium text-gray-50 group-hover:text-blue-300">{{ neighbors.prev.title }}</span>
        <span class="text-xs text-gray-500">{{ neighbors.prev.date }}</span>
      </RouterLink>
      <RouterLink v-if="neighbors.next" :to="to(neighbors.next)" rel="next" :class="[link, 'sm:col-start-2 sm:text-right']" data-neighbor="next">
        <span class="inline-flex items-center gap-1 text-xs text-gray-400 sm:justify-end">下一篇<ChevronRight class="size-4" /></span>
        <span class="truncate font-medium text-gray-50 group-hover:text-blue-300">{{ neighbors.next.title }}</span>
        <span class="text-xs text-gray-500">{{ neighbors.next.date }}</span>
      </RouterLink>
    </div>
  </nav>
</template>
