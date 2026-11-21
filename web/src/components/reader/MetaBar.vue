<script setup lang="ts">
import { Bot, CalendarDays, ExternalLink } from 'lucide-vue-next'
import { httpUrl } from '@/lib/url'
import ReadingControls from './ReadingControls.vue'

defineProps<{
  title: string
  summary?: string
  date?: string
  sourceAgent?: string
  tags?: string[]
  canonicalUrl?: string
}>()
</script>

<template>
  <header class="card mb-8 p-5 sm:p-6">
    <div v-if="$slots.crumbs" class="mb-3"><slot name="crumbs" /></div>
    <h1 class="text-2xl font-bold leading-tight text-gray-50 sm:text-3xl">{{ title }}</h1>
    <p v-if="summary" class="mt-3 text-gray-400">{{ summary }}</p>
    <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-sm text-gray-400">
      <span v-if="date" class="inline-flex items-center gap-1.5"><CalendarDays class="size-4" />{{ date }}</span>
      <span v-if="sourceAgent" class="inline-flex items-center gap-1.5"><Bot class="size-4" />{{ sourceAgent }}</span>
      <span v-for="t in tags" :key="t" class="chip">{{ t }}</span>
      <a v-if="httpUrl(canonicalUrl)" :href="httpUrl(canonicalUrl)" target="_blank" rel="noopener noreferrer" class="btn sm:ml-auto">
        <ExternalLink class="size-4" />原文链接
      </a>
    </div>
    <ReadingControls class="mt-4 border-t border-gray-700 pt-4" />
  </header>
</template>
