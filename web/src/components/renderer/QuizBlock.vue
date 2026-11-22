<script setup lang="ts">
import { computed } from 'vue'
import { CheckboxGroupRoot, CheckboxIndicator, CheckboxRoot, RadioGroupIndicator, RadioGroupItem, RadioGroupRoot } from 'reka-ui'
import { CircleCheck, CircleHelp, CircleX, RotateCcw, TriangleAlert } from 'lucide-vue-next'
import { children } from '@/lib/xml'
import { quizCorrect, quizKey, quizMulti, quizOptions, quizValid, sameSet } from '@/lib/quiz'
import { useReading } from '@/composables/useReading'

const props = defineProps<{ node: Element }>()
const reading = useReading()

const key = computed(() => quizKey(props.node))
const valid = computed(() => quizValid(props.node))
const multi = computed(() => quizMulti(props.node))
const correct = computed(() => quizCorrect(props.node))
const question = computed(() => children(props.node).find((c) => c.tagName === 'question')?.textContent?.trim() ?? '')
const explain = computed(() => children(props.node).find((c) => c.tagName === 'explain')?.textContent?.trim() ?? '')
// question, option and explanation text are plain text (rendered by Vue, escaped), never Markdown or HTML (PRD 6.7)
const options = computed(() => quizOptions(props.node).map((o, i) => ({ key: String(i), text: o.textContent?.trim() ?? '', right: o.getAttribute('correct') === 'true' })))

// what the Markdown export writes: question, options with the right ones ticked, and the explanation (shown on screen only after answering)
const exportMd = computed(() =>
  [
    `**随堂测验（${multi.value ? '多选' : '单选'}）**：${question.value}`,
    options.value.map((o) => `- [${o.right ? 'x' : ' '}] ${o.text}`).join('\n'),
    explain.value && `> 解析：${explain.value.replace(/\n/g, '\n> ')}`,
  ].filter(Boolean).join('\n\n'),
)

const st = computed(() => reading.quiz(key.value))
const picked = computed({
  get: () => st.value.picked,
  set: (v: string[]) => reading.pick(key.value, v),
})
const single = computed({
  get: () => st.value.picked[0],
  set: (v) => reading.pick(key.value, v ? [String(v)] : []),
})
const checked = computed(() => st.value.checked)
const passed = computed(() => checked.value && sameSet(st.value.picked, correct.value))

// per-option result once checked: ok = picked & right, bad = picked & wrong, missed = right but not picked
const result = (k: string, right: boolean) => {
  if (!checked.value) return ''
  const p = st.value.picked.includes(k)
  return p && right ? 'ok' : p ? 'bad' : right ? 'missed' : ''
}
const rowClass = (k: string, right: boolean) => [
  'flex w-full items-start gap-3 rounded-lg border px-3 py-2.5 text-left text-[0.95rem] outline-none transition focus-visible:ring-2 focus-visible:ring-blue-400',
  checked.value ? 'cursor-default' : 'cursor-pointer hover:bg-gray-700/40',
  {
    '': 'border-gray-600 text-gray-200 data-[state=checked]:border-blue-400 data-[state=checked]:bg-blue-500/10',
    ok: 'border-emerald-400 bg-emerald-500/15 text-gray-100',
    bad: 'border-red-400 bg-red-500/15 text-gray-100',
    missed: 'border-dashed border-emerald-400 bg-emerald-500/5 text-gray-200',
  }[result(k, right)],
]
</script>

<template>
  <section :id="`quiz-${key}`" class="not-prose scroll-mt-20 rounded-xl border border-violet-400/40 bg-violet-500/5 p-4 sm:p-5" :data-quiz="key" :data-export-md="valid ? exportMd : undefined" :data-result="checked ? (passed ? 'pass' : 'fail') : 'open'">
    <div class="mb-3 flex items-start gap-2">
      <CircleHelp class="mt-0.5 size-5 shrink-0 text-violet-300" aria-hidden="true" />
      <div class="min-w-0 flex-1">
        <p class="mb-1 flex items-center gap-2 text-xs font-semibold text-violet-300">
          随堂测验<span v-if="valid" class="rounded-full bg-violet-500/15 px-2 py-0.5 ring-1 ring-inset ring-violet-400/40">{{ multi ? '多选' : '单选' }}</span>
        </p>
        <p class="m-0 whitespace-pre-line break-words font-medium text-gray-50" data-quiz-question>{{ question }}</p>
      </div>
    </div>

    <p v-if="!valid" class="flex items-center gap-2 rounded-lg border border-amber-500/40 bg-amber-500/5 px-3 py-2 text-sm text-amber-300">
      <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />测验定义无效（INVALID_QUIZ：需要合法 id、非空题干、至少 2 个选项且至少 1 个 correct="true"），已跳过。
    </p>

    <template v-else>
      <RadioGroupRoot v-if="!multi" v-model="single" :disabled="checked" class="space-y-2" :aria-label="question">
        <RadioGroupItem v-for="o in options" :key="o.key" :value="o.key" :class="rowClass(o.key, o.right)" :data-result="result(o.key, o.right)">
          <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border border-gray-500" aria-hidden="true">
            <RadioGroupIndicator class="size-2 rounded-full bg-blue-400" />
          </span>
          <span class="min-w-0 flex-1 break-words">{{ o.text }}</span>
          <CircleCheck v-if="result(o.key, o.right) === 'ok' || result(o.key, o.right) === 'missed'" class="size-5 shrink-0 text-emerald-400" aria-label="正确选项" />
          <CircleX v-else-if="result(o.key, o.right) === 'bad'" class="size-5 shrink-0 text-red-400" aria-label="错误选项" />
        </RadioGroupItem>
      </RadioGroupRoot>

      <CheckboxGroupRoot v-else v-model="picked" :disabled="checked" class="space-y-2" :aria-label="question">
        <CheckboxRoot v-for="o in options" :key="o.key" :value="o.key" :class="rowClass(o.key, o.right)" :data-result="result(o.key, o.right)">
          <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded border border-gray-500" aria-hidden="true">
            <CheckboxIndicator class="text-[0.7rem] font-bold leading-none text-blue-400">✓</CheckboxIndicator>
          </span>
          <span class="min-w-0 flex-1 break-words">{{ o.text }}</span>
          <CircleCheck v-if="result(o.key, o.right) === 'ok' || result(o.key, o.right) === 'missed'" class="size-5 shrink-0 text-emerald-400" aria-label="正确选项" />
          <CircleX v-else-if="result(o.key, o.right) === 'bad'" class="size-5 shrink-0 text-red-400" aria-label="错误选项" />
        </CheckboxRoot>
      </CheckboxGroupRoot>

      <div class="mt-3 flex flex-wrap items-center gap-2">
        <button v-if="!checked" type="button" class="btn btn-primary" :disabled="st.picked.length === 0" data-action="check" @click="reading.check(key)">检查答案</button>
        <button v-else type="button" class="btn" data-action="retry" @click="reading.retry(key)"><RotateCcw class="size-4" aria-hidden="true" />重试</button>
        <p v-if="checked" role="status" :class="['flex items-center gap-1.5 text-sm font-semibold', passed ? 'text-emerald-300' : 'text-red-300']">
          <CircleCheck v-if="passed" class="size-4" aria-hidden="true" /><CircleX v-else class="size-4" aria-hidden="true" />
          {{ passed ? '回答正确' : multi ? '回答不完整或有误' : '回答错误' }}
        </p>
        <p v-else-if="multi" class="text-xs text-gray-400">可多选，全部选对才算正确</p>
      </div>

      <div v-if="checked && explain" class="mt-3 rounded-lg border border-gray-700 bg-gray-800/60 px-3 py-2.5" data-explain>
        <p class="mb-1 text-xs font-semibold text-gray-400">解析</p>
        <p class="m-0 whitespace-pre-line break-words text-sm text-gray-200">{{ explain }}</p>
      </div>
    </template>
  </section>
</template>
