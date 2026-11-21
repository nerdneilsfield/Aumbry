<script setup lang="ts">
import type { Component } from 'vue'
import SectionBlock from './SectionBlock.vue'
import MarkdownBlock from './MarkdownBlock.vue'
import FigureBlock from './FigureBlock.vue'
import FigureGroup from './FigureGroup.vue'
import PaperCard from './PaperCard.vue'
import RepoCard from './RepoCard.vue'
import CompareTable from './CompareTable.vue'
import CalloutBlock from './CalloutBlock.vue'
import CollapseBlock from './CollapseBlock.vue'
import SourcesBlock from './SourcesBlock.vue'
import ClaimBlock from './ClaimBlock.vue'
import EquationBlock from './EquationBlock.vue'
import MermaidBlock from './MermaidBlock.vue'
import ChartBlock from './ChartBlock.vue'
import MetricsBlock from './MetricsBlock.vue'
import StepsBlock from './StepsBlock.vue'
import TabsBlock from './TabsBlock.vue'
import CodeBlock from './CodeBlock.vue'
import QuoteBlock from './QuoteBlock.vue'
import ChecklistBlock from './ChecklistBlock.vue'
import TimelineBlock from './TimelineBlock.vue'
import GlossaryBlock from './GlossaryBlock.vue'
import ReferencesBlock from './ReferencesBlock.vue'

defineProps<{ node: Element }>()

// Resolved at render time: section/collapse/step/tab import this component back.
// MermaidBlock / ChartBlock are thin wrappers: mermaid and echarts themselves are dynamic imports behind them.
const block = (tag: string): Component | undefined => ({
  section: SectionBlock,
  figure: FigureBlock,
  'figure-group': FigureGroup,
  'paper-card': PaperCard,
  'repo-card': RepoCard,
  'compare-table': CompareTable,
  callout: CalloutBlock,
  collapse: CollapseBlock,
  sources: SourcesBlock,
  claim: ClaimBlock,
  equation: EquationBlock,
  mermaid: MermaidBlock,
  chart: ChartBlock,
  metrics: MetricsBlock,
  steps: StepsBlock,
  tabs: TabsBlock,
  'code-block': CodeBlock,
  quote: QuoteBlock,
  checklist: ChecklistBlock,
  timeline: TimelineBlock,
  glossary: GlossaryBlock,
  references: ReferencesBlock,
})[tag]
</script>

<template>
  <MarkdownBlock v-if="node.tagName === 'md'" :text="node.textContent ?? ''" />
  <!-- unknown tags are ignored on purpose -->
  <component :is="block(node.tagName)" v-else-if="block(node.tagName)" :node="node" />
</template>
