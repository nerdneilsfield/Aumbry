<script setup lang="ts">
import { computed } from 'vue'
import { ExternalLink, FolderGit2, GitCommitHorizontal, Scale, Star, Code } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { httpUrl } from '@/lib/url'
import MarkdownBlock from './MarkdownBlock.vue'

const props = defineProps<{ node: Element }>()
const url = computed(() => httpUrl(attr(props.node, 'url')))
const stars = computed(() => attr(props.node, 'stars'))
const lang = computed(() => attr(props.node, 'lang'))
const license = computed(() => attr(props.node, 'license'))
const commit = computed(() => attr(props.node, 'commit'))
</script>

<template>
  <article class="not-prose card border-l-4 border-l-emerald-400 p-5">
    <div class="flex items-start gap-3">
      <FolderGit2 class="mt-0.5 size-5 shrink-0 text-emerald-300" />
      <div class="min-w-0 flex-1">
        <h3 class="break-all font-mono text-base font-semibold text-gray-50">
          <a v-if="url" :href="url" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1.5 hover:text-emerald-300">
            {{ attr(node, 'name') }}<ExternalLink class="size-3.5 shrink-0" />
          </a>
          <template v-else>{{ attr(node, 'name') }}</template>
        </h3>
        <p v-if="stars || lang || license || commit" class="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-400">
          <span v-if="stars" class="inline-flex items-center gap-1"><Star class="size-3.5" />{{ stars }}</span>
          <span v-if="lang" class="inline-flex items-center gap-1"><Code class="size-3.5" />{{ lang }}</span>
          <span v-if="license" class="inline-flex items-center gap-1"><Scale class="size-3.5" />{{ license }}</span>
          <span v-if="commit" class="inline-flex items-center gap-1 font-mono"><GitCommitHorizontal class="size-3.5" />{{ commit.slice(0, 12) }}</span>
        </p>
        <MarkdownBlock class="mt-3 text-sm" :text="node.textContent ?? ''" />
      </div>
    </div>
  </article>
</template>
