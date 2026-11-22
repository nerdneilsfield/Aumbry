<script setup lang="ts">
import { computed } from 'vue'
import { ChevronsDownUp, ChevronsUpDown } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { useReading } from '@/composables/useReading'
import XmlNode from './XmlNode.vue'

/**
 * The children of a container (<body>, <section> or <stage>). Normally: its <tldr> first (or last with placement="bottom"),
 * then everything else in source order. In skim mode (PRD 9.7) the container's OWN blocks (md, figure, chart, quiz, ...) fold
 * away behind one button; its <tldr> stays, and so do child sections / stages groups, each of which folds its own blocks.
 */
const props = defineProps<{ node: Element; foldLabel: string; hintWhenNoTldr?: boolean }>()
const reading = useReading()

const kids = computed(() => children(props.node))
const tldr = computed(() => kids.value.find((c) => c.tagName === 'tldr'))
const bottom = computed(() => !!tldr.value && attr(tldr.value, 'placement') === 'bottom')
const isBlock = (c: Element) => !['tldr', 'section', 'stages'].includes(c.tagName)
const hasBlocks = computed(() => kids.value.some(isBlock))
const collapsed = computed(() => reading.isCollapsed(props.node))
const rest = computed(() => kids.value.filter((c) => c !== tldr.value))
const containers = computed(() => rest.value.filter((c) => !isBlock(c)))
</script>

<template>
  <template v-if="collapsed">
    <XmlNode v-if="tldr" :node="tldr" />
    <p v-else-if="hintWhenNoTldr && hasBlocks" class="not-prose rounded-lg border border-dashed border-gray-600 px-3 py-2 text-sm text-gray-500" data-no-tldr>这一部分没有 tldr，速读模式下只显示标题。</p>
    <div v-if="hasBlocks" class="print:hidden">
      <button type="button" class="btn" data-action="expand-unit" @click="reading.expand(node)"><ChevronsUpDown class="size-4" aria-hidden="true" />{{ foldLabel }}</button>
    </div>
    <XmlNode v-for="(n, i) in containers" :key="i" :node="n" />
  </template>
  <template v-else>
    <div v-if="reading.skim.value && hasBlocks" class="-mb-2 flex justify-end print:hidden">
      <button type="button" class="inline-flex cursor-pointer items-center gap-1 rounded-md px-2 py-1 text-xs text-gray-400 hover:bg-gray-700 hover:text-gray-100" data-action="collapse-unit" @click="reading.collapse(node)">
        <ChevronsDownUp class="size-3.5" aria-hidden="true" />收起
      </button>
    </div>
    <XmlNode v-if="tldr && !bottom" :node="tldr" />
    <XmlNode v-for="(n, i) in rest" :key="i" :node="n" />
    <XmlNode v-if="tldr && bottom" :node="tldr" />
  </template>
</template>
