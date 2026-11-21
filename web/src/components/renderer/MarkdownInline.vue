<script setup lang="ts">
import { computed } from 'vue'
import { renderInline } from '@/lib/markdown'

const props = defineProps<{ text: string }>()
// Same pipeline as <md> (markdown-it with html:false, then DOMPurify), inline rules only.
const html = computed(() => renderInline(props.text))
</script>

<template>
  <span class="[&_a]:text-blue-300 [&_a]:underline [&_a]:underline-offset-2 [&_code]:rounded [&_code]:bg-gray-700/70 [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em]" v-html="html" />
</template>
