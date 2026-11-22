<script setup lang="ts">
import { computed } from 'vue'
import { attr, children } from '@/lib/xml'
import { LEVELS, asLevel, type Level } from '@/lib/levels'

const props = defineProps<{ node: Element }>()
const text = (el: Element) => el.textContent?.trim() ?? ''
const cols = computed(() => Array.from(props.node.querySelectorAll('columns > col')).map(text))
const rows = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'row')
    .map((r) =>
      children(r)
        .filter((c) => c.tagName === 'cell')
        .map((c) => ({ text: text(c), tag: asLevel(attr(c, 'tag')) as Level | undefined })),
    ),
)
// Only tags that actually occur are listed in the legend.
const usedTags = computed(() => [...new Set(rows.value.flat().flatMap((c) => (c.tag ? [c.tag] : [])))])
</script>

<template>
  <div class="not-prose">
    <p v-if="attr(node, 'caption')" class="mb-2 text-sm font-medium text-gray-300">{{ attr(node, 'caption') }}</p>
    <div class="overflow-x-auto rounded-xl border border-gray-700">
      <table class="w-full border-collapse text-left text-sm">
        <thead class="bg-gray-800 text-gray-100">
          <tr>
            <th v-for="(c, i) in cols" :key="i" class="whitespace-nowrap border-b border-gray-700 px-4 py-2.5 font-semibold">{{ c }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-700/70">
          <tr v-for="(r, i) in rows" :key="i" class="odd:bg-gray-900 even:bg-gray-800/40">
            <td
              v-for="(cell, j) in r"
              :key="j"
              :class="[
                'relative min-w-32 px-4 py-2.5 align-top',
                cell.tag === 'na' ? 'bg-gray-500/15 text-gray-400' : cell.tag === 'unreported' ? 'border border-dashed border-gray-500 bg-transparent text-gray-500' : 'text-gray-300',
                cell.tag && 'pt-5',
              ]"
              :data-tag="cell.tag"
            >
              <span
                v-if="cell.tag"
                :data-export-badge="cell.tag !== 'unreported' ? '' : undefined"
                :data-export-skip="cell.tag === 'unreported' ? '' : undefined"
                :class="['absolute right-0 top-0 inline-flex items-center gap-0.5 rounded-bl-md px-1.5 py-px text-[0.65rem] font-semibold leading-4', LEVELS[cell.tag].badge]"
                :title="LEVELS[cell.tag].hint"
              >
                <component :is="LEVELS[cell.tag].icon" class="size-2.5" aria-hidden="true" />{{ LEVELS[cell.tag].label }}
              </span>
              <!-- unreported is never shown as a number: the cell says so, the original text is only a tooltip -->
              <span v-if="cell.tag === 'unreported'" class="italic" :title="cell.text || undefined">未报告</span>
              <span v-else :class="cell.tag === 'na' && 'italic'">{{ cell.text }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="usedTags.length" data-export-skip class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500">
      <span v-for="t in usedTags" :key="t" class="inline-flex items-center gap-1">
        <component :is="LEVELS[t].icon" class="size-3" aria-hidden="true" />{{ LEVELS[t].label }}：{{ LEVELS[t].hint }}
      </span>
    </p>
  </div>
</template>
