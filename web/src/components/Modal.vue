<script setup lang="ts">
import { DialogClose, DialogContent, DialogOverlay, DialogPortal, DialogRoot, DialogTitle } from 'reka-ui'
import { X } from 'lucide-vue-next'

defineProps<{ title: string; wide?: boolean }>()
const emit = defineEmits<{ close: [] }>()
</script>

<template>
  <DialogRoot :open="true" @update:open="(open) => !open && emit('close')">
    <DialogPortal>
      <DialogOverlay class="fixed inset-0 z-50 bg-black/60" />
      <DialogContent
        :aria-describedby="undefined"
        :class="['card fixed left-1/2 top-[8vh] z-50 max-h-[84vh] w-[calc(100%-2rem)] -translate-x-1/2 overflow-y-auto shadow-2xl', wide ? 'max-w-2xl' : 'max-w-md']"
      >
        <header class="flex items-center justify-between gap-4 border-b border-gray-700 px-5 py-3">
          <DialogTitle class="truncate font-semibold text-gray-50">{{ title }}</DialogTitle>
          <DialogClose class="cursor-pointer text-gray-400 hover:text-gray-100" aria-label="关闭"><X class="size-5" /></DialogClose>
        </header>
        <div class="p-5"><slot /></div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>
