<script setup lang="ts">
import { children } from '@/lib/xml'
import XmlNode from './XmlNode.vue'

defineProps<{ body: Element }>()
</script>

<template>
  <div class="space-y-10">
    <XmlNode v-for="(n, i) in children(body)" :key="i" :node="n" />
  </div>
</template>
