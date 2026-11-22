<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useExpanded } from '@/composables/usePrint'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ArrowRight, ChevronRight, ExternalLink, Flame, Newspaper, TriangleAlert } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { httpUrl } from '@/lib/url'
import { newsIssues } from '@/lib/validate'
import { DETAIL_NAV } from '@/composables/useDetailNav'
import MarkdownBlock from './MarkdownBlock.vue'
import MarkdownInline from './MarkdownInline.vue'

const props = defineProps<{ node: Element }>()
const detailNav = inject(DETAIL_NAV, null)

const issues = computed(() => newsIssues(props.node))
const title = computed(() => attr(props.node, 'title').trim())
const url = computed(() => httpUrl(props.node.getAttribute('url')))
const high = computed(() => attr(props.node, 'importance') === 'high')
const detailId = computed(() => attr(props.node, 'detail-id'))
const DETAIL_LABEL = '阅读详情'
// the Markdown export writes the jump to the detail entry as an absolute link (the saved file is read elsewhere)
const detailExportMd = computed(() => (detailId.value ? `[${DETAIL_LABEL}](${location.origin}/p/${encodeURIComponent(detailId.value)})` : undefined))
// the summary is the element's own text; an optional <detail> child is the inline long form
const summary = computed(() =>
  Array.from(props.node.childNodes)
    .filter((n) => n.nodeType === 3 || n.nodeType === 4)
    .map((n) => n.textContent ?? '')
    .join('')
    .trim(),
)
const detail = computed(() => children(props.node).find((c) => c.tagName === 'detail')?.textContent?.trim() ?? '')

const userOpen = ref(false)
const expanded = useExpanded()
const open = computed({ get: () => userOpen.value || expanded.value, set: (v) => (userOpen.value = v) })
</script>

<template>
  <div v-if="issues.length" class="not-prose rounded-xl border border-amber-500/40 bg-amber-500/5 px-4 py-3 text-sm text-amber-300" data-news-error role="alert">
    <p class="flex items-center gap-2 font-semibold"><TriangleAlert class="size-4 shrink-0" aria-hidden="true" />{{ [...new Set(issues.map((i) => i.code))].join(' / ') }}：这条新闻未渲染</p>
    <ul class="m-0 mt-1 list-disc space-y-0.5 pl-9 text-amber-200/90"><li v-for="(i, k) in issues" :key="k">{{ i.message }}</li></ul>
  </div>

  <article v-else :class="['not-prose card overflow-hidden border-l-4', high ? 'border-l-rose-400 bg-rose-500/5' : 'border-l-gray-600']" data-news :data-importance="high ? 'high' : 'normal'">
    <div class="px-4 py-3.5">
      <div class="flex flex-wrap items-center gap-x-2.5 gap-y-1 text-xs text-gray-400">
        <Newspaper class="size-3.5 shrink-0" aria-hidden="true" />
        <span v-if="attr(node, 'source')" class="font-medium text-gray-300">{{ attr(node, 'source') }}</span>
        <time v-if="attr(node, 'published')" class="font-mono">{{ attr(node, 'published') }}</time>
        <span v-if="high" class="inline-flex items-center gap-1 rounded-full bg-rose-500/15 px-2 py-0.5 font-semibold text-rose-300 ring-1 ring-inset ring-rose-400/40"><Flame class="size-3" aria-hidden="true" />重要</span>
      </div>
      <h3 class="mt-1.5 text-base font-semibold leading-snug text-gray-50">
        <a v-if="url" :href="url" target="_blank" rel="noopener noreferrer" class="inline-flex items-baseline gap-1 hover:text-blue-300">{{ title }}<ExternalLink class="size-3.5 shrink-0 self-center text-gray-500" aria-label="打开原文" /></a>
        <template v-else>{{ title }}</template>
      </h3>
      <p v-if="summary" class="mt-1.5 text-[0.95rem] leading-relaxed text-gray-300"><MarkdownInline :text="summary" /></p>

      <div v-if="detail || detailId" class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
        <CollapsibleRoot v-if="detail" v-model:open="open" class="w-full" data-news-detail>
          <CollapsibleTrigger class="inline-flex cursor-pointer items-center gap-1 text-sm font-medium text-blue-300 hover:text-blue-200 print:hidden">
            <ChevronRight :class="['size-4 transition-transform', open && 'rotate-90']" aria-hidden="true" />{{ open ? '收起详情' : '展开详情' }}
          </CollapsibleTrigger>
          <CollapsibleContent class="mt-2 rounded-lg border border-gray-700 bg-gray-900/40 px-4 py-3 text-sm"><MarkdownBlock :text="detail" /></CollapsibleContent>
        </CollapsibleRoot>
        <component
          :is="detailNav ? 'button' : RouterLink"
          v-if="detailId"
          v-bind="detailNav ? { type: 'button', onClick: () => detailNav!(detailId) } : { to: `/p/${encodeURIComponent(detailId)}` }"
          class="inline-flex cursor-pointer items-center gap-1 text-sm font-semibold text-blue-300 hover:text-blue-200 print:hidden"
          :data-detail-id="detailId"
          :data-export-md="detailExportMd"
        >{{ DETAIL_LABEL }}<ArrowRight class="size-4" aria-hidden="true" /></component>
      </div>
    </div>
  </article>
</template>
