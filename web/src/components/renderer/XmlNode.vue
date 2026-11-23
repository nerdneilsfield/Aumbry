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
import StagesBlock from './StagesBlock.vue'
import QuizBlock from './QuizBlock.vue'
import NextSteps from './NextSteps.vue'
import SearchLog from './SearchLog.vue'
import TldrBlock from './TldrBlock.vue'
import NewsItem from './NewsItem.vue'
import AudioBlock from './AudioBlock.vue'
import DuplicateId from './DuplicateId.vue'
import { useAnchors } from '@/composables/useAnchors'

defineProps<{ node: Element }>()
const anchors = useAnchors()

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
  stages: StagesBlock,
  quiz: QuizBlock,
  'next-steps': NextSteps,
  'search-log': SearchLog,
  tldr: TldrBlock,
  'news-item': NewsItem,
  audio: AudioBlock,
})[tag]
</script>

<template>
  <MarkdownBlock v-if="node.tagName === 'md'" :text="node.textContent ?? ''" />
  <!-- unknown tags are ignored on purpose -->
  <!-- a stage / stages group reports its own duplicate ids (StagesBlock) -->
  <DuplicateId v-else-if="anchors.isDuplicate(node) && node.tagName !== 'stages'" :node="node" />
  <component :is="block(node.tagName)" v-else-if="block(node.tagName)" :node="node" />
</template>
