<script setup lang="ts">
import { SwitchRoot, SwitchThumb } from 'reka-ui'
import { readerBridge } from '@/composables/useReading'

// Page-level reading switches, living in the header and talking to the renderer on screen through readerBridge:
// skim mode (only when the document has a tldr) and "expand all" (one switch for every <stages> group).
const track = 'relative h-5 w-9 shrink-0 rounded-full border border-gray-600 bg-gray-700 outline-none transition focus-visible:ring-2 focus-visible:ring-blue-400 data-[state=checked]:border-blue-400 data-[state=checked]:bg-blue-600'
const thumb = 'block size-4 translate-x-0.5 rounded-full bg-white transition-transform data-[state=checked]:translate-x-[1.1rem]'
</script>

<template>
  <div v-if="readerBridge && (readerBridge.hasTldr.value || readerBridge.hasStages.value)" class="flex flex-wrap items-center gap-x-5 gap-y-2 print:hidden" data-reading-controls>
    <label v-if="readerBridge.hasTldr.value" class="flex cursor-pointer items-center gap-2 text-sm text-gray-300" title="只看各部分的标题与要点，需要时再展开">
      <SwitchRoot v-model="readerBridge.skim.value" :class="track" data-skim-toggle><SwitchThumb :class="thumb" /></SwitchRoot>
      速读
    </label>
    <label v-if="readerBridge.hasStages.value" class="flex cursor-pointer items-center gap-2 text-sm text-gray-300" title="打开后所有分阶段内容的各阶段都直接可见">
      <SwitchRoot v-model="readerBridge.expandAll.value" :class="track" data-expand-all><SwitchThumb :class="thumb" /></SwitchRoot>
      展开全部
    </label>
  </div>
</template>
