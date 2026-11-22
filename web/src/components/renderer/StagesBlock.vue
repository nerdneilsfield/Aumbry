<script setup lang="ts">
import { computed } from 'vue'
import {
  ProgressIndicator, ProgressRoot, StepperDescription, StepperIndicator, StepperItem, StepperRoot, StepperSeparator,
  StepperTitle, StepperTrigger,
} from 'reka-ui'
import { ArrowRight, Check, CircleCheck, Lock, TriangleAlert, Trophy } from 'lucide-vue-next'
import { attr } from '@/lib/xml'
import { stageDomId } from '@/lib/anchor'
import { stagesIssues } from '@/lib/validate'
import { useReading } from '@/composables/useReading'
import ContainerBody from './ContainerBody.vue'

const props = defineProps<{ node: Element }>()
const reading = useReading()

// A group is rendered only when it is well-formed; otherwise an inline error stands in its place (the rest of the page is unaffected).
const issues = computed(() => stagesIssues(props.node))
const group = computed(() => reading.groupOf(props.node))
const g = computed(() => group.value!)

const stages = computed(() =>
  (group.value?.stages ?? []).map((el, i) => ({
    el,
    index: i + 1,
    domId: stageDomId(el),
    n: attr(el, 'n') || String(i + 1),
    title: attr(el, 'title'),
    level: attr(el, 'level'),
  })),
)
const last = computed(() => stages.value.length)
// A single-stage group has nothing to pace: no stepper, no "continue" - just the stage, expanded.
const single = computed(() => last.value === 1)
const open = computed(() => !!group.value && reading.isOpen(group.value))
const codes = computed(() => [...new Set(issues.value.map((i) => i.code))].join(' / '))
</script>

<template>
  <div v-if="issues.length" class="not-prose rounded-xl border border-amber-500/40 bg-amber-500/5 px-4 py-3 text-sm text-amber-300" data-stages-error role="alert">
    <p class="flex items-center gap-2 font-semibold"><TriangleAlert class="size-4 shrink-0" aria-hidden="true" />{{ codes }}：这组阶段未渲染</p>
    <ul class="m-0 mt-1 list-disc space-y-0.5 pl-9 text-amber-200/90"><li v-for="(i, k) in issues" :key="k">{{ i.message }}</li></ul>
  </div>

  <div v-else :id="`stages-${node.getAttribute('id')}`" :data-stages="node.getAttribute('id')" :data-single="single || undefined">
    <!-- sticky stepper + progress (hidden when printing: print expands every stage) -->
    <div v-if="!single" class="sticky top-0 z-20 -mx-4 border-b border-gray-700 bg-gray-900/90 px-4 pb-2 pt-2 backdrop-blur sm:mx-0 sm:rounded-b-xl sm:px-3 print:hidden" data-stage-bar>
      <StepperRoot :model-value="reading.current(g)" :linear="false" class="flex items-center" aria-label="阅读阶段" @update:model-value="(v) => v && reading.goTo(g, v)">
        <StepperItem
          v-for="s in stages"
          :key="s.index"
          v-slot="{ state }"
          :step="s.index"
          :completed="reading.isDone(g, s.index)"
          :disabled="!reading.isUnlocked(g, s.index)"
          :class="['flex items-center', s.index < last ? 'min-w-0 flex-1' : 'min-w-0 shrink']"
          :data-stage-step="s.index"
        >
          <StepperTrigger
            class="group/t flex min-w-0 cursor-pointer items-center gap-2 rounded-lg p-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-blue-400 disabled:cursor-not-allowed"
            :title="`第 ${s.n} 阶段 · ${s.title}${s.level ? '（' + s.level + '）' : ''}`"
          >
            <StepperIndicator
              :class="[
                'flex size-8 shrink-0 items-center justify-center rounded-full border text-sm font-bold tabular-nums transition',
                state === 'active' ? 'border-blue-400 bg-blue-600 text-white' : state === 'completed' ? 'border-emerald-400 bg-emerald-500/15 text-emerald-300' : 'border-gray-600 bg-gray-800 text-gray-500',
              ]"
            >
              <Check v-if="state === 'completed'" class="size-4" aria-hidden="true" />
              <Lock v-else-if="!reading.isUnlocked(g, s.index)" class="size-3.5" aria-hidden="true" />
              <template v-else>{{ s.index }}</template>
            </StepperIndicator>
            <span class="hidden min-w-0 group-data-[state=active]/t:block sm:block">
              <StepperTitle :class="['block truncate text-sm font-semibold', state === 'inactive' ? 'text-gray-400' : 'text-gray-50']">{{ s.title }}</StepperTitle>
              <StepperDescription v-if="s.level" class="block truncate text-xs text-gray-500">{{ s.level }}</StepperDescription>
            </span>
          </StepperTrigger>
          <StepperSeparator v-if="s.index < last" :class="['mx-2 h-px min-w-3 flex-1', reading.isDone(g, s.index) ? 'bg-emerald-400/60' : 'bg-gray-700']" />
        </StepperItem>
      </StepperRoot>

      <div class="mt-1.5 flex items-center gap-3">
        <ProgressRoot :model-value="reading.progress(g)" class="relative h-1.5 flex-1 overflow-hidden rounded-full bg-gray-700" aria-label="阅读进度">
          <ProgressIndicator class="size-full rounded-full bg-blue-400 transition-transform duration-500" :style="{ transform: `translateX(-${100 - reading.progress(g)}%)` }" />
        </ProgressRoot>
        <span class="shrink-0 text-xs tabular-nums text-gray-400" data-progress>{{ reading.progress(g) }}%</span>
      </div>
    </div>

    <div :class="['space-y-10', !single && 'mt-6']">
      <template v-for="s in stages" :key="s.index">
        <article v-if="reading.isUnlocked(g, s.index)" :id="s.domId" class="scroll-mt-24 space-y-8" :data-stage="s.index">
          <header class="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border border-blue-400/30 bg-blue-500/5 px-4 py-3">
            <span v-if="!single" data-export-skip class="rounded-full bg-blue-500/15 px-2.5 py-0.5 text-xs font-bold text-blue-300 ring-1 ring-inset ring-blue-400/40">第 {{ s.n }} 阶段</span>
            <h2 class="min-w-0 flex-1 text-lg font-bold text-gray-50">{{ s.title }}</h2>
            <span v-if="s.level" class="rounded-md border border-gray-600 px-2 py-0.5 text-xs text-gray-300">{{ s.level }}</span>
          </header>

          <ContainerBody :node="s.el" fold-label="展开本阶段" hint-when-no-tldr />

          <footer v-if="reading.score(s.el).total || (!single && (s.index === last || !open))" class="space-y-3 rounded-xl border border-gray-700 bg-gray-800/60 px-4 py-3" :data-stage-footer="s.index">
            <p v-if="reading.score(s.el).total" class="flex items-center gap-2 text-sm font-semibold text-gray-200" :data-score="s.index">
              <Trophy class="size-4 text-amber-400" aria-hidden="true" />
              本阶段测验：答对 {{ reading.score(s.el).correct }}/{{ reading.score(s.el).total }}
              <span class="font-normal text-gray-400">（已作答 {{ reading.score(s.el).answered }}/{{ reading.score(s.el).total }}）</span>
            </p>
            <div v-if="!single" class="flex flex-wrap items-center gap-2 print:hidden">
              <template v-if="s.index < last">
                <button v-if="!open" type="button" class="btn btn-primary" data-action="continue" @click="reading.goTo(g, s.index + 1)">
                  继续：{{ stages[s.index].title }}<ArrowRight class="size-4" aria-hidden="true" />
                </button>
              </template>
              <template v-else>
                <button v-if="!reading.isFinished(g)" type="button" class="btn btn-primary" data-action="finish" @click="reading.finish(g)">
                  <CircleCheck class="size-4" aria-hidden="true" />完成阅读
                </button>
                <p v-else class="flex items-center gap-1.5 text-sm font-semibold text-emerald-300"><CircleCheck class="size-4" aria-hidden="true" />已完成阅读</p>
              </template>
            </div>
          </footer>
        </article>

        <div v-else class="flex items-center gap-3 rounded-xl border border-dashed border-gray-600 px-4 py-5 text-gray-400 print:hidden" :data-stage-locked="s.index">
          <Lock class="size-5 shrink-0" aria-hidden="true" />
          <div class="min-w-0">
            <p class="font-semibold text-gray-300">第 {{ s.n }} 阶段：{{ s.title }}<span v-if="s.level" class="ml-2 text-xs font-normal text-gray-500">{{ s.level }}</span></p>
            <p class="text-sm">读完上一阶段后点击「继续」解锁；也可以打开页面顶部的「展开全部」。</p>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
