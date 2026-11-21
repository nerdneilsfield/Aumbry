<script setup lang="ts">
import { computed } from 'vue'
import { renderMarkdown } from '@/lib/markdown'

const props = defineProps<{ text: string }>()
// Safe for v-html: markdown-it runs with html:false and validated links.
const html = computed(() => renderMarkdown(props.text))
</script>

<template>
  <div class="prose prose-invert max-w-none prose-a:text-blue-300 prose-headings:text-gray-100" v-html="html" />
</template>
