<script setup lang="ts">
import { computed } from 'vue'
import { BookA } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
const terms = computed(() => children(props.node).filter((c) => c.tagName === 'term'))
</script>

<template>
  <div class="not-prose card p-4 sm:p-5">
    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold text-gray-300">
      <BookA class="size-4 text-gray-500" aria-hidden="true" />{{ attr(node, 'title') || '术语表' }}
    </h3>
    <dl class="divide-y divide-gray-700/60">
      <div v-for="(t, i) in terms" :key="i" class="grid gap-x-6 gap-y-1 py-3 first:pt-0 last:pb-0 sm:grid-cols-[minmax(7rem,12rem)_1fr]">
        <dt class="break-words font-semibold text-gray-50">{{ attr(t, 'name') }}</dt>
        <dd class="m-0 min-w-0">
          <MarkdownBlock class="md-tight text-sm" :text="t.textContent ?? ''" />
        </dd>
      </div>
    </dl>
  </div>
</template>
