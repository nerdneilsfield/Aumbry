<script setup lang="ts">
import { AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogOverlay, AlertDialogPortal, AlertDialogRoot, AlertDialogTitle } from 'reka-ui'

defineProps<{ title: string; description: string; confirmText?: string; pending?: boolean }>()
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ confirm: [] }>()
</script>

<template>
  <!-- Closing is refused while the action runs; the confirm button is a plain button so the dialog stays open until the caller closes it. -->
  <AlertDialogRoot :open="open" @update:open="(o) => (o || !pending) && (open = o)">
    <AlertDialogPortal>
      <AlertDialogOverlay class="fixed inset-0 z-[60] bg-black/60" />
      <AlertDialogContent class="card fixed left-1/2 top-[20vh] z-[60] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 p-5 shadow-2xl">
        <AlertDialogTitle class="font-semibold text-gray-50">{{ title }}</AlertDialogTitle>
        <AlertDialogDescription class="mt-2 text-sm text-gray-400">{{ description }}</AlertDialogDescription>
        <div class="mt-6 flex justify-end gap-2">
          <AlertDialogCancel class="btn" :disabled="pending">取消</AlertDialogCancel>
          <button class="btn btn-danger" :disabled="pending" @click="emit('confirm')">{{ pending ? '删除中…' : (confirmText ?? '确认删除') }}</button>
        </div>
      </AlertDialogContent>
    </AlertDialogPortal>
  </AlertDialogRoot>
</template>
