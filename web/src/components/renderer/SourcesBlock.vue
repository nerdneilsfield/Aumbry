<script setup lang="ts">
import { Link2 } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { httpUrl } from '@/lib/url'

const props = defineProps<{ node: Element }>()
const sources = Array.from(props.node.querySelectorAll('source')).map((s) => ({
  title: attr(s, 'title'),
  url: httpUrl(attr(s, 'url')),
}))
</script>

<template>
  <div class="not-prose card p-4">
    <h3 class="mb-2 text-sm font-semibold text-gray-300">参考来源</h3>
    <ul class="space-y-1.5 text-sm">
      <li v-for="(s, i) in sources" :key="i" class="flex items-center gap-2">
        <Link2 class="size-4 shrink-0 text-gray-500" />
        <a v-if="s.url" :href="s.url" target="_blank" rel="noopener noreferrer" class="break-all text-blue-300 hover:underline">{{ s.title || s.url }}</a>
        <span v-else class="text-gray-400">{{ s.title }}</span>
      </li>
    </ul>
  </div>
</template>
