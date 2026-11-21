<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ChevronRight, CircleSlash, Search, SquareCheck, SquareX, TriangleAlert } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { searchLogIssues } from '@/lib/validate'

const props = defineProps<{ node: Element }>()
const text = (el: Element) => el.textContent?.trim() ?? ''
const all = (tag: string) => children(props.node).filter((c) => c.tagName === tag)

const queries = computed(() => all('query').map((q) => ({ source: attr(q, 'source'), date: attr(q, 'date'), hits: Number(attr(q, 'hits')) || 0, kept: Number(attr(q, 'kept')) || 0, q: text(q) })))
const excluded = computed(() => all('excluded').map((e) => ({ title: attr(e, 'title'), reason: attr(e, 'reason') })))
const gaps = computed(() => all('gap').map(text))
const stop = computed(() => all('stop').map(text)[0] ?? '')
const crit = computed(() => {
  const c = all('criteria')[0]
  const kids = c ? children(c) : []
  return {
    include: kids.filter((k) => k.tagName === 'include').map(text),
    exclude: kids.filter((k) => k.tagName === 'exclude').map(text),
    note: attr(props.node, 'criteria') || (c && !kids.length ? text(c) : ''),
  }
})
const totals = computed(() => ({ hits: queries.value.reduce((n, q) => n + q.hits, 0), kept: queries.value.reduce((n, q) => n + q.kept, 0) }))
const issues = computed(() => searchLogIssues(props.node))

// default-collapsed audit card; printing shows it open
const userOpen = ref(false)
const printing = useMediaQuery('print')
const open = computed({ get: () => userOpen.value || printing.value, set: (v) => (userOpen.value = v) })
</script>

<template>
  <CollapsibleRoot v-model:open="open" class="not-prose card overflow-hidden" data-search-log>
    <CollapsibleTrigger class="flex w-full cursor-pointer flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-left hover:bg-gray-700/40">
      <ChevronRight :class="['size-4 shrink-0 text-gray-400 transition-transform', open && 'rotate-90']" />
      <Search class="size-4 shrink-0 text-gray-400" aria-hidden="true" />
      <span class="font-medium text-gray-100">检索记录</span>
      <span v-if="attr(node, 'date')" class="font-mono text-xs text-gray-400">{{ attr(node, 'date') }}</span>
      <span class="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-gray-400" data-search-counts>
        <span>{{ queries.length }} 条检索式</span><span>命中 {{ totals.hits }} · 保留 {{ totals.kept }}</span><span>排除 {{ excluded.length }}</span><span>空白 {{ gaps.length }}</span>
      </span>
      <TriangleAlert v-if="issues.length" class="size-4 shrink-0 text-amber-300" aria-label="检索记录有问题" />
    </CollapsibleTrigger>
    <CollapsibleContent class="space-y-4 border-t border-gray-700 p-4 text-sm">
      <p v-for="(i, k) in issues" :key="k" class="flex items-center gap-2 rounded-lg border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-amber-300">
        <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />{{ i.code }}：{{ i.message }}
      </p>

      <div v-if="crit.include.length || crit.exclude.length || crit.note" class="grid gap-3 sm:grid-cols-2">
        <p v-if="crit.note" class="text-gray-300 sm:col-span-2"><span class="font-semibold text-gray-100">筛选标准：</span>{{ crit.note }}</p>
        <div v-if="crit.include.length">
          <p class="mb-1 text-xs font-semibold text-emerald-300">纳入</p>
          <ul class="m-0 list-none space-y-1 p-0">
            <li v-for="(c, i) in crit.include" :key="i" class="flex gap-2 text-gray-300"><SquareCheck class="mt-0.5 size-4 shrink-0 text-emerald-400" aria-hidden="true" />{{ c }}</li>
          </ul>
        </div>
        <div v-if="crit.exclude.length">
          <p class="mb-1 text-xs font-semibold text-red-300">排除</p>
          <ul class="m-0 list-none space-y-1 p-0">
            <li v-for="(c, i) in crit.exclude" :key="i" class="flex gap-2 text-gray-300"><SquareX class="mt-0.5 size-4 shrink-0 text-red-400" aria-hidden="true" />{{ c }}</li>
          </ul>
        </div>
      </div>

      <div v-if="queries.length" class="overflow-x-auto rounded-lg border border-gray-700">
        <table class="w-full border-collapse text-left text-xs">
          <thead class="bg-gray-900/60 text-gray-300">
            <tr><th class="px-3 py-2">来源</th><th class="px-3 py-2">日期</th><th class="px-3 py-2">检索式</th><th class="px-3 py-2 text-right">命中</th><th class="px-3 py-2 text-right">保留</th></tr>
          </thead>
          <tbody class="divide-y divide-gray-700/70">
            <tr v-for="(q, i) in queries" :key="i">
              <td class="whitespace-nowrap px-3 py-2 text-gray-200">{{ q.source }}</td>
              <td class="whitespace-nowrap px-3 py-2 font-mono text-gray-400">{{ q.date }}</td>
              <td class="px-3 py-2 font-mono text-gray-300">{{ q.q }}</td>
              <td class="px-3 py-2 text-right tabular-nums text-gray-300">{{ q.hits }}</td>
              <td class="px-3 py-2 text-right tabular-nums font-semibold text-gray-100">{{ q.kept }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="excluded.length">
        <p class="mb-1 text-xs font-semibold text-gray-400">被排除的条目（{{ excluded.length }}）</p>
        <ul class="m-0 list-none space-y-1.5 p-0">
          <li v-for="(e, i) in excluded" :key="i" class="flex gap-2 text-gray-300"><CircleSlash class="mt-0.5 size-4 shrink-0 text-gray-500" aria-hidden="true" /><span><span class="text-gray-100">{{ e.title }}</span><span class="text-gray-400"> — {{ e.reason }}</span></span></li>
        </ul>
      </div>

      <div v-if="gaps.length" class="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3">
        <p class="mb-1 text-xs font-semibold text-amber-300">空白与盲区</p>
        <ul class="m-0 list-disc space-y-1 pl-5 text-gray-300"><li v-for="(g, i) in gaps" :key="i">{{ g }}</li></ul>
      </div>

      <p v-if="stop" class="border-t border-gray-700 pt-3 text-gray-300"><span class="font-semibold text-gray-100">停止规则：</span>{{ stop }}</p>
    </CollapsibleContent>
  </CollapsibleRoot>
</template>
