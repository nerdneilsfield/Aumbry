<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ExternalLink, FileText, BookOpen, Code } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { httpUrl } from '@/lib/url'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
const url = computed(() => httpUrl(attr(props.node, 'url')))
const relatedId = computed(() => attr(props.node, 'related-id'))
const meta = computed(() => [attr(props.node, 'authors'), attr(props.node, 'venue'), attr(props.node, 'date')].filter(Boolean))
const codeUrl = computed(() => httpUrl(attr(props.node, 'code-url')))
// v1.1: doi is not a URL; validated against the PRD pattern and linked through doi.org.
const doi = computed(() => (/^10\.\d{4,9}\/[-._;()/:A-Za-z0-9]+$/.test(attr(props.node, 'doi')) ? attr(props.node, 'doi') : ''))
const depth = computed(() => ({ full: '全文精读', abstract: '仅读摘要', snippet: '仅读片段' })[attr(props.node, 'read-depth')])
</script>

<template>
  <article class="not-prose card border-l-4 border-l-blue-400 p-5">
    <div class="flex items-start gap-3">
      <FileText class="mt-1 size-5 shrink-0 text-blue-300" />
      <div class="min-w-0 flex-1">
        <h3 class="text-base font-semibold leading-snug text-gray-50">
          <a v-if="url" :href="url" target="_blank" rel="noopener noreferrer" class="hover:text-blue-300">{{ attr(node, 'title') }}</a>
          <template v-else>{{ attr(node, 'title') }}</template>
        </h3>
        <p v-if="meta.length" class="mt-1 text-xs text-gray-400">{{ meta.join(' · ') }}</p>
        <p v-if="depth" class="mt-2">
          <span :class="['rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset', depth === '全文精读' ? 'bg-emerald-500/15 text-emerald-300 ring-emerald-400/40' : 'bg-amber-500/15 text-amber-300 ring-amber-400/40']">{{ depth }}</span>
        </p>
        <MarkdownBlock class="mt-3 text-sm" :text="node.textContent ?? ''" />
        <div v-if="url || relatedId || codeUrl || doi" class="mt-4 flex flex-wrap gap-2">
          <RouterLink v-if="relatedId" :to="`/p/${encodeURIComponent(relatedId)}`" class="btn btn-primary">
            <BookOpen class="size-4" />查看精读
          </RouterLink>
          <a v-if="url" :href="url" target="_blank" rel="noopener noreferrer" class="btn">
            <ExternalLink class="size-4" />原文
          </a>
          <a v-if="codeUrl" :href="codeUrl" target="_blank" rel="noopener noreferrer" class="btn">
            <Code class="size-4" />代码
          </a>
          <a v-if="doi" :href="`https://doi.org/${doi}`" target="_blank" rel="noopener noreferrer" class="btn font-mono">
            DOI
          </a>
        </div>
      </div>
    </div>
  </article>
</template>
