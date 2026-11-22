<script setup lang="ts">
import { computed } from 'vue'
import katex from 'katex'
import DOMPurify from 'dompurify'
import { TriangleAlert } from 'lucide-vue-next'
import { attr, safeId } from '@/lib/xml'
import 'katex/dist/katex.min.css'

const props = defineProps<{ node: Element }>()

const src = computed(() => (props.node.textContent ?? '').trim())
// throwOnError:false renders broken LaTeX as red source text instead of throwing; trust:false disables \href, \includegraphics etc.
// KaTeX output is sanitised again before it is mounted (same rule as the markdown pipeline).
const html = computed(() =>
  DOMPurify.sanitize(
    katex.renderToString(src.value, { displayMode: true, throwOnError: false, trust: false, strict: 'ignore', maxExpand: 1000, maxSize: 30 }),
  ),
)
const broken = computed(() => html.value.includes('katex-error'))
const anchor = computed(() => (safeId(attr(props.node, 'id')) ? `eq-${attr(props.node, 'id')}` : undefined))
const number = computed(() => attr(props.node, 'number'))
</script>

<template>
  <figure :id="anchor" class="not-prose scroll-mt-20 rounded-xl border border-gray-700 bg-gray-800/60 px-4 py-4">
    <div class="grid grid-cols-[1fr_auto] items-center gap-x-4">
      <!-- eslint-disable-next-line vue/no-v-html -- KaTeX output (trust:false), re-sanitised by DOMPurify above -->
      <div class="min-w-0 overflow-x-auto overflow-y-hidden py-1 text-lg" :data-tex="src" v-html="html" />
      <span v-if="number" class="font-serif text-base text-gray-300">({{ number }})</span>
    </div>
    <p v-if="broken" class="mt-2 flex items-center gap-1.5 text-xs text-red-300">
      <TriangleAlert class="size-3.5 shrink-0" aria-hidden="true" />LaTeX 语法有误，已按原文显示。
    </p>
    <figcaption v-if="attr(node, 'caption')" class="mt-2 text-center text-sm text-gray-400">{{ attr(node, 'caption') }}</figcaption>
  </figure>
</template>
