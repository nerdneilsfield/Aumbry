<script setup lang="ts">
import { computed } from 'vue'
import { useClipboard } from '@vueuse/core'
import { Check, Copy, FileCode2, GitCommitHorizontal, Sigma } from 'lucide-vue-next'
import { attr, safeId } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'
import { highlightCode } from '@/lib/code'

const props = defineProps<{ node: Element }>()

// CDATA code usually starts right after a newline: drop the first and trailing blank lines only, keep indentation.
const code = computed(() => (props.node.textContent ?? '').replace(/^\r?\n/, '').replace(/\s+$/, ''))
const lang = computed(() => attr(props.node, 'lang').toLowerCase())
const diff = computed(() => attr(props.node, 'diff') === 'true' || lang.value === 'diff')
const lines = computed(() => attr(props.node, 'lines'))
// `lines="66-78"`: the excerpt's line range in the file. Line numbers start at its first number.
const start = computed(() => {
  const n = Number.parseInt(lines.value, 10)
  return Number.isFinite(n) && n > 0 && n < 1e7 ? n : 1
})
// shiki escapes the code itself; the HTML is shiki's own output.
const html = computed(() => highlightCode(code.value, lang.value, diff.value, attr(props.node, 'highlight')))
const commit = computed(() => attr(props.node, 'commit').slice(0, 12))
// the Markdown export names the block after its file (and line range)
const exportTitle = computed(() => (attr(props.node, 'file') ? attr(props.node, 'file') + (lines.value ? ` (L${lines.value})` : '') : undefined))

const { copy, copied } = useClipboard({ copiedDuring: 1500, legacy: true })

// paper-to-code: `eq="<equation id>"` links this code to the equation it implements.
const { canTarget, navigate } = useAnchors()
// `eq` is the raw equation id (no # and no eq- prefix)
const eqId = computed(() => (safeId(attr(props.node, 'eq')) ? attr(props.node, 'eq') : ''))
const eqLabel = computed(() => {
  const eq = Array.from(props.node.ownerDocument.querySelectorAll('equation')).find((e) => e.getAttribute('id') === eqId.value)
  const n = eq?.getAttribute('number')
  return n ? `对应公式 (${n})` : '对应公式'
})
const eqOk = computed(() => !!eqId.value && canTarget(`#eq-${eqId.value}`))
</script>

<template>
  <figure class="not-prose" :data-export-title="exportTitle">
    <div class="overflow-hidden rounded-xl border border-gray-700">
      <div data-export-skip class="flex min-h-10 items-center gap-2 border-b border-gray-700 bg-gray-800 px-3 py-1.5 text-xs text-gray-400">
        <FileCode2 class="size-4 shrink-0" aria-hidden="true" />
        <span v-if="attr(node, 'file')" class="min-w-0 truncate font-mono text-gray-200">{{ attr(node, 'file') }}</span>
        <span v-if="lines" class="shrink-0 font-mono">L{{ lines }}</span>
        <button
          v-if="attr(node, 'eq')"
          type="button"
          :disabled="!eqOk"
          :title="eqOk ? `跳到公式 #eq-${eqId}` : '对应的公式不存在'"
          data-eq-chip
          class="inline-flex shrink-0 cursor-pointer items-center gap-1 rounded-full bg-violet-500/15 px-2 py-0.5 font-sans font-semibold text-violet-300 ring-1 ring-inset ring-violet-400/40 hover:bg-violet-500/25 disabled:cursor-not-allowed disabled:opacity-50"
          @click="navigate(`#eq-${eqId}`)"
        ><Sigma class="size-3" aria-hidden="true" />{{ eqLabel }}</button>
        <span v-if="commit" class="inline-flex shrink-0 items-center gap-0.5 font-mono"><GitCommitHorizontal class="size-3.5" aria-hidden="true" />{{ commit }}</span>
        <span class="ml-auto flex shrink-0 items-center gap-2">
          <span v-if="lang && !diff" class="rounded bg-gray-700/70 px-1.5 py-0.5 font-mono uppercase tracking-wide text-gray-300">{{ lang }}</span>
          <span v-if="diff" class="rounded bg-amber-500/15 px-1.5 py-0.5 font-mono uppercase tracking-wide text-amber-300">diff</span>
          <button
            type="button"
            class="inline-flex size-7 cursor-pointer items-center justify-center rounded-md text-gray-400 transition hover:bg-gray-700 hover:text-gray-100 print:hidden"
            :aria-label="copied ? '已复制' : '复制代码'"
            :title="copied ? '已复制' : '复制代码'"
            @click="copy(code)"
          >
            <Check v-if="copied" class="size-4 text-emerald-400" />
            <Copy v-else class="size-4" />
          </button>
        </span>
      </div>
      <!-- eslint-disable-next-line vue/no-v-html -- shiki output (escapes code) -->
      <div class="code-block" :style="{ counterReset: `ln ${start - 1}` }" v-html="html" />
    </div>
    <figcaption v-if="attr(node, 'caption')" class="mt-2 text-center text-sm text-gray-400">{{ attr(node, 'caption') }}</figcaption>
  </figure>
</template>
