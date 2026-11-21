<script setup lang="ts">
import { Check, Link } from 'lucide-vue-next'
import { useClipboard } from '@vueuse/core'
import { attr, children, sectionId } from '@/lib/xml'
import XmlNode from './XmlNode.vue'

const props = defineProps<{ node: Element }>()
const { copy, copied } = useClipboard({ copiedDuring: 1500, legacy: true })
// Hash is appended to whatever page the reader is on (works for /r/:id, /p/:id and the offline demo).
const copyLink = () => copy(`${location.href.replace(/#.*$/, '')}#${sectionId(props.node)}`)
</script>

<template>
  <section :id="sectionId(node)" class="scroll-mt-20 space-y-5">
    <h2 class="group flex items-center gap-2 border-b border-gray-700 pb-2 text-xl font-semibold text-gray-50">
      <span class="min-w-0">{{ attr(node, 'title') }}</span>
      <button
        type="button"
        class="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-gray-500 opacity-0 transition hover:bg-gray-700 hover:text-gray-100 focus-visible:opacity-100 group-hover:opacity-100 max-lg:opacity-60"
        :aria-label="copied ? '已复制' : '复制章节链接'"
        :title="copied ? '已复制' : '复制章节链接'"
        @click="copyLink"
      >
        <Check v-if="copied" class="size-4 text-emerald-400" />
        <Link v-else class="size-4" />
      </button>
    </h2>
    <XmlNode v-for="(n, i) in children(node)" :key="i" :node="n" />
  </section>
</template>
