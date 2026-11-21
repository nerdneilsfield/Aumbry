<script setup lang="ts">
import { attr, children, sectionId } from '@/lib/xml'
import XmlNode from './XmlNode.vue'

defineProps<{ node: Element }>()
</script>

<template>
  <section :id="sectionId(node)" class="scroll-mt-6 space-y-5">
    <h2 class="border-b border-gray-700 pb-2 text-xl font-semibold text-gray-50">{{ attr(node, 'title') }}</h2>
    <XmlNode v-for="(n, i) in children(node)" :key="i" :node="n" />
  </section>
</template>
