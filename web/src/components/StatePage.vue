<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{ icon: Component; title: string; message?: string }>()
</script>

<template>
  <div class="mx-auto flex max-w-md flex-col items-center py-20 text-center">
    <div class="mb-5 rounded-full border border-gray-700 bg-gray-800 p-4 text-gray-400">
      <component :is="icon" class="size-8" />
    </div>
    <h1 class="text-xl font-semibold text-gray-50">{{ title }}</h1>
    <p v-if="message" class="mt-2 text-sm text-gray-400">{{ message }}</p>
    <div class="mt-6 flex w-full justify-center gap-2"><slot /></div>
  </div>
</template>
