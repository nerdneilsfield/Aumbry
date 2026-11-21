<script setup lang="ts">
import { computed } from 'vue'
import { ExternalLink, Quote } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { httpUrl } from '@/lib/url'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
const url = computed(() => httpUrl(attr(props.node, 'url')))
const byline = computed(() => [attr(props.node, 'author'), attr(props.node, 'source')].filter(Boolean))
</script>

<template>
  <!-- verbatim excerpt: serif + tinted surface, clearly different from the analysis prose around it -->
  <figure class="not-prose relative rounded-xl border border-gray-700 bg-gray-800/50 py-4 pl-12 pr-5">
    <Quote class="absolute left-4 top-4 size-5 text-gray-500" aria-hidden="true" />
    <blockquote class="m-0 font-serif">
      <MarkdownBlock class="md-tight italic" :text="node.textContent ?? ''" />
    </blockquote>
    <figcaption v-if="byline.length || url" class="mt-3 flex flex-wrap items-center gap-x-2 text-xs text-gray-400">
      <span v-if="byline.length">— {{ byline.join(' · ') }}</span>
      <a v-if="url" :href="url" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 text-blue-300 hover:underline">
        <ExternalLink class="size-3" aria-hidden="true" />原文
      </a>
    </figcaption>
  </figure>
</template>
