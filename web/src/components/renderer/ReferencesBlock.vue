<script setup lang="ts">
import { computed } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import { attr, children, safeId } from '@/lib/xml'
import { httpUrl } from '@/lib/url'

const props = defineProps<{ node: Element }>()
// doi is not a URL: it is validated against the PRD pattern and then turned into a doi.org link.
const DOI = /^10\.\d{4,9}\/[-._;()/:A-Za-z0-9]+$/
const refs = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'ref')
    .map((r) => ({
      anchor: safeId(attr(r, 'id')) ? `ref-${attr(r, 'id')}` : undefined,
      title: attr(r, 'title'),
      authors: attr(r, 'authors'),
      year: attr(r, 'year'),
      venue: attr(r, 'venue'),
      url: httpUrl(attr(r, 'url')),
      doi: DOI.test(attr(r, 'doi')) ? attr(r, 'doi') : '',
    })),
)
</script>

<template>
  <div class="not-prose card p-4 sm:p-5">
    <h3 class="mb-3 text-sm font-semibold text-gray-300">{{ attr(node, 'title') || '参考文献' }}</h3>
    <ol class="m-0 list-none space-y-2.5 p-0 text-sm">
      <li v-for="(r, i) in refs" :id="r.anchor" :key="i" class="flex scroll-mt-20 gap-3 rounded-md">
        <span class="w-8 shrink-0 text-right font-mono text-gray-500 tabular-nums">[{{ i + 1 }}]</span>
        <p class="m-0 min-w-0 flex-1 leading-relaxed text-gray-300">
          <span v-if="r.authors">{{ r.authors.replace(/\.+$/, '') }}. </span>
          <a v-if="r.url" :href="r.url" target="_blank" rel="noopener noreferrer" class="font-medium text-blue-300 hover:underline">{{ r.title }}</a>
          <span v-else class="font-medium text-gray-100">{{ r.title }}</span>
          <span v-if="r.venue || r.year" class="text-gray-400">. <i>{{ r.venue }}</i><template v-if="r.venue && r.year">, </template>{{ r.year }}</span>
          <a v-if="r.doi" :href="`https://doi.org/${r.doi}`" target="_blank" rel="noopener noreferrer" class="ml-2 inline-flex items-center gap-0.5 font-mono text-xs text-gray-400 hover:text-blue-300">
            doi:{{ r.doi }}<ExternalLink class="size-3" aria-hidden="true" />
          </a>
        </p>
      </li>
    </ol>
  </div>
</template>
