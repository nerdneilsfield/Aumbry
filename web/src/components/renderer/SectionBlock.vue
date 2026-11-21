<script setup lang="ts">
import { computed } from 'vue'
import { Check, Link, TriangleAlert } from 'lucide-vue-next'
import { useClipboard } from '@vueuse/core'
import { MAX_SECTION_DEPTH, attr, sectionDepth, sectionId } from '@/lib/xml'
import ContainerBody from './ContainerBody.vue'

const props = defineProps<{ node: Element }>()
const { copy, copied } = useClipboard({ copiedDuring: 1500, legacy: true })
// Hash is appended to whatever page the reader is on (works for /r/:id, /p/:id and the offline demo).
const copyLink = () => copy(`${location.href.replace(/#.*$/, '')}#${sectionId(props.node)}`)

// Sections nest up to 3 levels (only <section> ancestors count, PRD 6.0); a 4th level is shown as an error instead of being rendered.
const depth = computed(() => sectionDepth(props.node))
const tooDeep = computed(() => depth.value > MAX_SECTION_DEPTH)

const heading = computed(() => `h${Math.min(depth.value + 1, 4)}`)
const headingClass = computed(() => ['border-b border-gray-700 pb-2 text-xl', 'text-lg', 'text-base'][depth.value - 1] ?? 'text-base')
</script>

<template>
  <div v-if="tooDeep" class="not-prose rounded-xl border border-amber-500/40 bg-amber-500/5 px-4 py-3 text-sm text-amber-300" data-section-error role="alert">
    <p class="flex items-center gap-2 font-semibold"><TriangleAlert class="size-4 shrink-0" aria-hidden="true" />SECTION_TOO_DEEP：section 最多嵌套 {{ MAX_SECTION_DEPTH }} 层</p>
    <p class="mt-1 pl-6 text-amber-200/90">「{{ attr(node, 'title') }}」是第 {{ depth }} 层，这一层及其内容未渲染。把它提升一层，或改用 <code>&lt;collapse&gt;</code>、<code>&lt;steps&gt;</code>。</p>
  </div>

  <section
    v-else
    :id="sectionId(node)"
    :class="['scroll-mt-20 space-y-5', depth > 1 && 'border-l border-gray-700 pl-4 sm:pl-5']"
    :data-section-depth="depth"
  >
    <component :is="heading" :class="['group flex items-center gap-2 font-semibold text-gray-50', headingClass]">
      <span class="min-w-0">{{ attr(node, 'title') }}</span>
      <button
        type="button"
        class="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-gray-500 opacity-0 transition hover:bg-gray-700 hover:text-gray-100 focus-visible:opacity-100 group-hover:opacity-100 max-lg:opacity-60 print:hidden"
        :aria-label="copied ? '已复制' : '复制章节链接'"
        :title="copied ? '已复制' : '复制章节链接'"
        @click="copyLink"
      >
        <Check v-if="copied" class="size-4 text-emerald-400" />
        <Link v-else class="size-4" />
      </button>
    </component>

    <ContainerBody :node="node" fold-label="展开本节" hint-when-no-tldr />
  </section>
</template>
