<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { DialogClose, DialogContent, DialogDescription, DialogOverlay, DialogPortal, DialogRoot, DialogTitle, DialogTrigger } from 'reka-ui'
import { ListTree, Lock, X } from 'lucide-vue-next'
import type { TocEntry } from '@/lib/xml'
import { readerBridge } from '@/composables/useReading'

defineProps<{ toc: TocEntry[]; active: string }>()
const emit = defineEmits<{ jump: [id: string] }>()

const open = ref(false)
// Entries of a stage the reader has not unlocked yet are marked; clicking one still jumps there (and unlocks it).
const locked = (t: TocEntry) => !!t.stage && !!readerBridge.value && !readerBridge.value.isUnlocked(t.group, t.stage)
// The stage each group is at (the one its step bar shows as active), marked apart from the scroll highlight (PRD 6.7).
const current = (t: TocEntry) => t.kind === 'stage' && !!readerBridge.value && readerBridge.value.currentStage(t.group) === t.stage
const badge = 'shrink-0 self-center rounded-full bg-blue-500/15 px-1.5 text-[0.65rem] font-semibold leading-4 text-blue-300 ring-1 ring-inset ring-blue-400/40'
const indent = (t: TocEntry) => ({ paddingLeft: `${(t.level - 1) * 0.75}rem` })
// Close the drawer first (it locks body scroll), then scroll.
async function jumpFromDrawer(id: string) {
  open.value = false
  await nextTick()
  setTimeout(() => emit('jump', id), 60)
}
</script>

<template>
  <!-- >= lg: sticky side column -->
  <aside v-if="toc.length" class="hidden lg:block print:hidden">
    <nav class="sticky top-6 text-sm" aria-label="目录">
      <h2 class="mb-3 text-xs font-semibold uppercase tracking-wider text-gray-500">目录</h2>
      <ul class="space-y-0.5 border-l border-gray-700" data-toc="desktop">
        <li v-for="t in toc" :key="t.id" :style="indent(t)">
          <a
            :href="`#${t.id}`"
            :data-toc-id="t.id"
            :data-toc-kind="t.kind"
            :aria-current="current(t) ? 'step' : undefined"
            :data-toc-current="current(t) || undefined"
            :class="[
              '-ml-px block border-l-2 py-1 pl-3 transition',
              t.kind === 'stage' ? 'mt-2 font-semibold' : '',
              active === t.id ? 'border-blue-400 text-blue-300' : 'border-transparent hover:text-gray-100 ' + (t.kind === 'stage' ? 'text-gray-200' : 'text-gray-400'),
            ]"
            @click.prevent="emit('jump', t.id)"
          >
            <span class="flex items-baseline gap-1.5"><Lock v-if="locked(t)" class="size-3 shrink-0 self-center text-gray-500" aria-label="未解锁" /><span class="min-w-0">{{ t.title }}</span><span v-if="current(t)" :class="badge">当前</span></span>
            <span v-if="t.summary" class="mt-0.5 line-clamp-2 block text-xs font-normal leading-snug text-gray-500" data-toc-summary>{{ t.summary }}</span>
          </a>
        </li>
      </ul>
    </nav>
  </aside>

  <!-- < lg: floating button + Reka Dialog drawer (PRD 9.2) -->
  <DialogRoot v-if="toc.length" v-model:open="open">
    <DialogTrigger
      class="fixed bottom-5 right-5 z-30 inline-flex size-12 cursor-pointer items-center justify-center rounded-full border border-blue-400/50 bg-blue-600 text-white shadow-lg shadow-black/30 transition hover:bg-blue-500 lg:hidden print:hidden"
      aria-label="打开章节目录"
    >
      <ListTree class="size-5" />
    </DialogTrigger>
    <DialogPortal>
      <DialogOverlay class="toc-overlay print:hidden fixed inset-0 z-40 bg-black/50 backdrop-blur-[2px]" />
      <DialogContent class="toc-drawer print:hidden fixed inset-y-0 right-0 z-50 flex w-80 max-w-[85vw] flex-col border-l border-gray-700 bg-gray-800 shadow-2xl outline-none">
        <div class="flex items-center justify-between border-b border-gray-700 px-4 py-3">
          <DialogTitle class="text-sm font-semibold text-gray-100">章节目录</DialogTitle>
          <DialogClose class="inline-flex size-8 cursor-pointer items-center justify-center rounded-md text-gray-400 hover:bg-gray-700 hover:text-gray-100" aria-label="关闭目录">
            <X class="size-4" />
          </DialogClose>
        </div>
        <DialogDescription class="sr-only">选择章节跳转到对应位置</DialogDescription>
        <nav class="min-h-0 flex-1 overflow-y-auto p-3" aria-label="章节目录">
          <ul class="space-y-1" data-toc="drawer">
            <li v-for="t in toc" :key="t.id" :style="indent(t)">
              <a
                :href="`#${t.id}`"
                :data-toc-id="t.id"
                :aria-current="current(t) ? 'step' : undefined"
                :data-toc-current="current(t) || undefined"
                :class="[
                  'block rounded-lg px-3 py-2.5 text-sm transition',
                  t.kind === 'stage' ? 'mt-1 font-semibold' : '',
                  active === t.id ? 'bg-blue-500/15 font-medium text-blue-300' : 'text-gray-300 hover:bg-gray-700',
                ]"
                @click.prevent="jumpFromDrawer(t.id)"
              >
                <span class="flex items-baseline gap-2"><Lock v-if="locked(t)" class="size-3 shrink-0 self-center text-gray-500" aria-label="未解锁" /><span class="min-w-0">{{ t.title }}</span><span v-if="current(t)" :class="badge">当前</span></span>
                <span v-if="t.summary" class="mt-0.5 line-clamp-2 block text-xs font-normal leading-snug text-gray-500" data-toc-summary>{{ t.summary }}</span>
              </a>
            </li>
          </ul>
        </nav>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style>
.toc-drawer[data-state='open'] {
  animation: toc-in 0.2s ease-out;
}
.toc-drawer[data-state='closed'] {
  animation: toc-out 0.15s ease-in;
}
.toc-overlay[data-state='open'] {
  animation: toc-fade-in 0.2s ease-out;
}
.toc-overlay[data-state='closed'] {
  animation: toc-fade-out 0.15s ease-in;
}
@keyframes toc-in {
  from { transform: translateX(100%); }
}
@keyframes toc-out {
  to { transform: translateX(100%); }
}
@keyframes toc-fade-in {
  from { opacity: 0; }
}
@keyframes toc-fade-out {
  to { opacity: 0; }
}
</style>
