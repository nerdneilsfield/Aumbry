<script setup lang="ts">
import { computed } from 'vue'
import { attr, children } from '@/lib/xml'

const props = defineProps<{ node: Element }>()
const text = (el: Element) => el.textContent?.trim() ?? ''
const cols = computed(() => Array.from(props.node.querySelectorAll('columns > col')).map(text))
const rows = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'row')
    .map((r) => children(r).filter((c) => c.tagName === 'cell').map(text)),
)
</script>

<template>
  <div class="not-prose">
    <p v-if="attr(node, 'caption')" class="mb-2 text-sm font-medium text-gray-300">{{ attr(node, 'caption') }}</p>
    <div class="overflow-x-auto rounded-xl border border-gray-700">
      <table class="w-full border-collapse text-left text-sm">
        <thead class="bg-gray-800 text-gray-100">
          <tr>
            <th v-for="(c, i) in cols" :key="i" class="whitespace-nowrap px-4 py-2.5 font-semibold">{{ c }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-700/70">
          <tr v-for="(r, i) in rows" :key="i" class="odd:bg-gray-900 even:bg-gray-800/40">
            <td v-for="(cell, j) in r" :key="j" class="px-4 py-2.5 align-top text-gray-300">{{ cell }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
