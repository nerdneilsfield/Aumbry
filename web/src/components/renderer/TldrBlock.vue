<script setup lang="ts">
import { computed } from 'vue'
import { Gavel, ListChecks, TriangleAlert } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { tldrIssues } from '@/lib/validate'
import MarkdownInline from './MarkdownInline.vue'

const props = defineProps<{ node: Element }>()

const issues = computed(() => tldrIssues(props.node))
const whole = computed(() => props.node.parentElement?.tagName === 'body')
const title = computed(() => attr(props.node, 'title').trim() || (whole.value ? '全文要点' : '本节要点'))
const points = computed(() => children(props.node).filter((c) => c.tagName === 'point').map((p) => p.textContent?.trim() ?? ''))
const verdict = computed(() => children(props.node).find((c) => c.tagName === 'verdict')?.textContent?.trim() ?? '')
</script>

<template>
  <!-- a bad tldr never hides the rest of the document: it is replaced by a readable error right where it stands -->
  <div v-if="issues.length" class="not-prose rounded-xl border border-amber-500/40 bg-amber-500/5 px-4 py-3 text-sm text-amber-300" data-tldr-error role="alert">
    <p class="flex items-center gap-2 font-semibold"><TriangleAlert class="size-4 shrink-0" aria-hidden="true" />INVALID_TLDR：这个要点卡片未渲染</p>
    <ul class="m-0 mt-1 list-disc space-y-0.5 pl-9 text-amber-200/90"><li v-for="(i, k) in issues" :key="k">{{ i.message }}</li></ul>
  </div>

  <aside
    v-else
    :class="['not-prose rounded-xl border px-4 py-3.5', whole ? 'border-blue-400/40 bg-blue-500/5' : 'border-sky-400/25 bg-sky-500/5']"
    :data-tldr="whole ? 'whole' : 'part'"
    :aria-label="title"
  >
    <p :class="['mb-2 flex items-center gap-2 text-xs font-semibold tracking-wide', whole ? 'text-blue-300' : 'text-sky-300']">
      <ListChecks class="size-4" aria-hidden="true" />{{ title }}
    </p>
    <ul class="m-0 list-none space-y-1.5 p-0">
      <li v-for="(p, i) in points" :key="i" class="flex gap-2.5 text-[0.95rem] leading-relaxed text-gray-200" data-tldr-point>
        <span :class="['mt-[0.6em] size-1.5 shrink-0 rounded-full', whole ? 'bg-blue-400' : 'bg-sky-400']" aria-hidden="true" />
        <MarkdownInline class="min-w-0 break-words" :text="p" />
      </li>
    </ul>
    <p v-if="verdict" class="mt-3 flex gap-2 rounded-lg border border-amber-400/40 bg-amber-500/10 px-3 py-2 text-[0.95rem] font-medium leading-relaxed text-amber-100" data-tldr-verdict>
      <Gavel class="mt-1 size-4 shrink-0 text-amber-300" aria-hidden="true" />
      <MarkdownInline class="min-w-0 break-words" :text="verdict" />
    </p>
  </aside>
</template>
