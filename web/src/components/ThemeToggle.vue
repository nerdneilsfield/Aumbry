<script setup lang="ts">
import { Moon, Sun } from 'lucide-vue-next'
import { useTheme } from '@/composables/useTheme'

const { isDark, toggle } = useTheme()
</script>

<template>
  <button
    type="button"
    class="btn size-9 !p-0"
    :aria-label="isDark ? '切换到浅色主题' : '切换到深色主题'"
    :title="isDark ? '浅色主题' : '深色主题'"
    @click="toggle"
  >
    <Sun v-if="isDark" class="size-4" />
    <Moon v-else class="size-4" />
  </button>
</template>
