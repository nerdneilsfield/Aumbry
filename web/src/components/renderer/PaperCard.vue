<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ExternalLink, FileText, BookOpen } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { httpUrl } from '@/lib/url'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
const url = httpUrl(attr(props.node, 'url'))
const relatedId = attr(props.node, 'related-id')
const meta = [attr(props.node, 'authors'), attr(props.node, 'date')].filter(Boolean)
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
        <MarkdownBlock class="mt-3 text-sm" :text="node.textContent ?? ''" />
        <div v-if="url || relatedId" class="mt-4 flex flex-wrap gap-2">
          <RouterLink v-if="relatedId" :to="`/p/${encodeURIComponent(relatedId)}`" class="btn btn-primary">
            <BookOpen class="size-4" />查看精读
          </RouterLink>
          <a v-if="url" :href="url" target="_blank" rel="noopener noreferrer" class="btn">
            <ExternalLink class="size-4" />原文
          </a>
        </div>
      </div>
    </div>
  </article>
</template>
