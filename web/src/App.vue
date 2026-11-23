<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { Library } from 'lucide-vue-next'
import { api } from '@/api/client'
import { formatVersionSummary } from '@/lib/version'
import ThemeToggle from '@/components/ThemeToggle.vue'

const route = useRoute()

const versionQuery = useQuery({
  queryKey: ['server-version'],
  queryFn: api.version,
  staleTime: Infinity,
  retry: false,
})
const versionSummary = computed(() => (versionQuery.data.value ? formatVersionSummary(versionQuery.data.value) : null))
</script>

<template>
  <div class="mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 sm:px-6">
    <header class="flex h-16 items-center gap-3 border-b border-gray-700 print:hidden">
      <RouterLink to="/" class="inline-flex items-center gap-2 text-lg font-bold text-gray-50">
        <Library class="size-5 text-blue-300" />Aumbry
      </RouterLink>
      <span class="hidden text-sm text-gray-500 sm:inline">Research, gathered and searchable.</span>
      <ThemeToggle class="ml-auto" />
    </header>
    <main class="flex-1 py-8">
      <!-- keyed by path: moving to another directory starts a fresh page, so a half-typed search of the old one cannot land in the new URL -->
      <RouterView :key="route.path" />
    </main>
    <footer
      v-if="versionSummary"
      data-testid="app-version"
      class="mt-auto flex flex-wrap items-center justify-between gap-2 border-t border-gray-700/60 py-4 text-xs text-gray-500 print:hidden"
    >
      <span>Aumbry {{ versionSummary.version }}</span>
      <span class="flex items-center gap-3 font-mono tabular-nums" :title="versionSummary.rawBuildTime">
        <span>commit {{ versionSummary.commit }}</span>
        <span>built {{ versionSummary.buildTime }}</span>
      </span>
    </footer>
  </div>
</template>
