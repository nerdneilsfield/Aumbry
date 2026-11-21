<script setup lang="ts">
import type { Component } from 'vue'
import SectionBlock from './SectionBlock.vue'
import MarkdownBlock from './MarkdownBlock.vue'
import FigureBlock from './FigureBlock.vue'
import PaperCard from './PaperCard.vue'
import RepoCard from './RepoCard.vue'
import CompareTable from './CompareTable.vue'
import CalloutBlock from './CalloutBlock.vue'
import CollapseBlock from './CollapseBlock.vue'
import SourcesBlock from './SourcesBlock.vue'

defineProps<{ node: Element }>()

// Resolved at render time: section/collapse import this component back.
const block = (tag: string): Component | undefined => ({
  section: SectionBlock,
  figure: FigureBlock,
  'paper-card': PaperCard,
  'repo-card': RepoCard,
  'compare-table': CompareTable,
  callout: CalloutBlock,
  collapse: CollapseBlock,
  sources: SourcesBlock,
})[tag]
</script>

<template>
  <MarkdownBlock v-if="node.tagName === 'md'" :text="node.textContent ?? ''" />
  <!-- unknown tags are ignored on purpose -->
  <component :is="block(node.tagName)" v-else-if="block(node.tagName)" :node="node" />
</template>
