<script setup lang="ts">
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Library } from 'lucide-vue-next'
import ThemeToggle from '@/components/ThemeToggle.vue'

const route = useRoute()
</script>

<template>
  <div class="mx-auto w-full max-w-7xl px-4 sm:px-6">
    <header class="flex h-16 items-center gap-3 border-b border-gray-700 print:hidden">
      <RouterLink to="/" class="inline-flex items-center gap-2 text-lg font-bold text-gray-50">
        <Library class="size-5 text-blue-300" />Aumbry
      </RouterLink>
      <span class="hidden text-sm text-gray-500 sm:inline">Research, gathered and searchable.</span>
      <ThemeToggle class="ml-auto" />
    </header>
    <main class="py-8">
      <!-- keyed by path: moving to another directory starts a fresh page, so a half-typed search of the old one cannot land in the new URL -->
      <RouterView :key="route.path" />
    </main>
  </div>
</template>
