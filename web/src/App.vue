<script setup lang="ts">
import { RouterLink, RouterView } from 'vue-router'
import { Library } from 'lucide-vue-next'
</script>

<template>
  <div class="mx-auto w-full max-w-7xl px-4 sm:px-6">
    <header class="flex h-16 items-center gap-3 border-b border-gray-800">
      <RouterLink to="/" class="inline-flex items-center gap-2 text-lg font-bold text-gray-50">
        <Library class="size-5 text-blue-300" />Aumbry
      </RouterLink>
      <span class="hidden text-sm text-gray-500 sm:inline">Research, gathered and searchable.</span>
    </header>
    <main class="py-8">
      <RouterView />
    </main>
  </div>
</template>
