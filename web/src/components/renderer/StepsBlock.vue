<script setup lang="ts">
import { computed, useTemplateRef } from 'vue'
import { onKeyStroke, useMediaQuery } from '@vueuse/core'
import { StepperIndicator, StepperItem, StepperRoot, StepperSeparator, StepperTitle, StepperTrigger } from 'reka-ui'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { attr, children } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'
import XmlNode from './XmlNode.vue'

const props = defineProps<{ node: Element }>()
const { activeSteps } = useAnchors()
// Printing mounts every step; otherwise only the active one is in the DOM.
const printing = useMediaQuery('print')
const steps = computed(() => children(props.node).filter((c) => c.tagName === 'step'))
const interactive = computed(() => attr(props.node, 'interactive') === 'true' && steps.value.length > 1)

// Interactive player: one step at a time. The active step lives in the shared anchor state so that
// #eq-* / #fig-* links into a hidden step can switch to it. Only the active step is mounted (all of them while printing).
const step = computed({
  get: () => Math.min((activeSteps.get(props.node) ?? 0) + 1, steps.value.length),
  set: (n: number) => activeSteps.set(props.node, Math.min(Math.max(n, 1), steps.value.length) - 1),
})

const player = useTemplateRef<HTMLElement>('player')
// Arrow keys step through the player while focus is inside it (the dots keep Reka's own roving focus, so they are skipped).
const arrow = (d: number) => (e: KeyboardEvent) => {
  if ((e.target as Element | null)?.closest('[data-stepper-dot]')) return
  e.preventDefault()
  step.value += d
}
onKeyStroke('ArrowRight', arrow(1), { target: player })
onKeyStroke('ArrowLeft', arrow(-1), { target: player })
</script>

<template>
  <!-- static: numbered vertical list -->
  <ol v-if="!interactive" class="not-prose list-none p-0">
    <li v-for="(s, i) in steps" :key="i" class="relative flex gap-4 pb-6 last:pb-0">
      <span v-if="i < steps.length - 1" class="absolute bottom-0 left-4 top-9 w-px -translate-x-1/2 bg-gray-700" aria-hidden="true" />
      <span class="relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full border border-blue-400/60 bg-blue-500/15 text-sm font-bold text-blue-300 tabular-nums">{{ i + 1 }}</span>
      <div class="min-w-0 flex-1 space-y-3 pt-0.5">
        <h3 class="text-base font-semibold leading-8 text-gray-50">{{ attr(s, 'title') }}</h3>
        <XmlNode v-for="(n, j) in children(s)" :key="j" :node="n" />
      </div>
    </li>
  </ol>

  <!-- interactive: stepper player -->
  <div v-else ref="player" tabindex="-1" class="not-prose card overflow-hidden outline-none" data-steps-player role="group" aria-label="分步演示，左右方向键切换">
    <StepperRoot v-model="step" :linear="false" class="border-b border-gray-700 bg-gray-900/50 px-4 py-3 print:hidden">
      <div class="flex items-center">
        <StepperItem v-for="(s, i) in steps" :key="i" :step="i + 1" :class="['flex items-center', i < steps.length - 1 ? 'min-w-0 flex-1' : '']">
          <StepperTrigger
            data-stepper-dot
            :aria-label="`第 ${i + 1} 步：${attr(s, 'title')}`"
            class="group/d flex cursor-pointer flex-col items-center gap-1 rounded-md p-0.5 outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
          >
            <StepperIndicator
              class="flex size-7 items-center justify-center rounded-full border border-gray-600 bg-gray-800 text-xs font-bold tabular-nums text-gray-400 transition group-data-[state=active]/d:border-blue-400 group-data-[state=active]/d:bg-blue-600 group-data-[state=active]/d:text-white group-data-[state=completed]/d:border-emerald-400 group-data-[state=completed]/d:text-emerald-300"
            >{{ i + 1 }}</StepperIndicator>
            <StepperTitle class="hidden max-w-24 truncate text-[0.7rem] text-gray-400 group-data-[state=active]/d:text-gray-100 sm:block">{{ attr(s, 'title') }}</StepperTitle>
          </StepperTrigger>
          <StepperSeparator v-if="i < steps.length - 1" class="mx-1 h-px min-w-2 flex-1 bg-gray-700 data-[state=completed]:bg-emerald-400/60" />
        </StepperItem>
      </div>
    </StepperRoot>

    <div class="p-4 sm:p-5">
      <template v-for="(s, i) in steps" :key="i">
        <div v-if="i + 1 === step || printing" class="space-y-3" :data-step-panel="i + 1">
        <h3 class="flex items-baseline gap-2 text-base font-semibold text-gray-50">
          <span class="rounded-md bg-blue-500/15 px-1.5 py-0.5 font-mono text-xs text-blue-300">{{ i + 1 }}/{{ steps.length }}</span>{{ attr(s, 'title') }}
        </h3>
        <XmlNode v-for="(n, j) in children(s)" :key="j" :node="n" />
        </div>
      </template>
    </div>

    <div class="flex items-center justify-between gap-3 border-t border-gray-700 bg-gray-900/40 px-4 py-2.5 print:hidden">
      <button type="button" class="btn" :disabled="step <= 1" data-action="prev" @click="step--"><ChevronLeft class="size-4" aria-hidden="true" />上一步</button>
      <span class="text-xs tabular-nums text-gray-400" aria-live="polite">第 {{ step }} / {{ steps.length }} 步</span>
      <button type="button" class="btn btn-primary" :disabled="step >= steps.length" data-action="next" @click="step++">下一步<ChevronRight class="size-4" aria-hidden="true" /></button>
    </div>
  </div>
</template>
