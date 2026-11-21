<script setup lang="ts">
import { SelectContent, SelectItem, SelectItemText, SelectPortal, SelectRoot, SelectTrigger, SelectValue, SelectViewport } from 'reka-ui'
import { Check, ChevronDown } from 'lucide-vue-next'

// Option values must be non-empty strings (Reka Select restriction).
defineProps<{ options: { value: string; label: string; depth?: number }[]; label?: string }>()
const model = defineModel<string>({ required: true })
</script>

<template>
  <SelectRoot v-model="model">
    <SelectTrigger class="input flex cursor-pointer items-center justify-between gap-2 text-left" :aria-label="label">
      <SelectValue class="truncate" />
      <ChevronDown class="size-4 shrink-0 text-gray-500" />
    </SelectTrigger>
    <SelectPortal>
      <SelectContent position="popper" :side-offset="4" class="z-[60] max-h-72 min-w-(--reka-select-trigger-width) overflow-hidden rounded-lg border border-gray-600 bg-gray-800 shadow-xl">
        <SelectViewport class="p-1">
          <SelectItem
            v-for="o in options"
            :key="o.value"
            :value="o.value"
            class="flex cursor-pointer items-center justify-between gap-3 rounded px-2 py-1.5 text-sm text-gray-200 outline-none data-highlighted:bg-gray-700"
            :style="{ paddingLeft: `${0.5 + (o.depth ?? 0) * 1}rem` }"
          >
            <SelectItemText>{{ o.label }}</SelectItemText>
            <Check v-if="model === o.value" class="size-4 text-blue-300" />
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>
