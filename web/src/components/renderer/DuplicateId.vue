<script setup lang="ts">
import { TriangleAlert } from 'lucide-vue-next'
import { attr } from '@/lib/xml'

// One id namespace for section / stages / stage / quiz / figure / equation / ref (PRD 6.4): the first element keeps the id,
// a later one - even of another type - is shown as this error instead of being rendered.
defineProps<{ node: Element }>()
</script>

<template>
  <div class="not-prose rounded-xl border border-amber-500/40 bg-amber-500/5 px-4 py-3 text-sm text-amber-300" data-dup-error role="alert">
    <p class="flex items-center gap-2 font-semibold"><TriangleAlert class="size-4 shrink-0" aria-hidden="true" />DUPLICATE_ID：id「{{ attr(node, 'id') }}」重复</p>
    <p class="mt-1 pl-6 text-amber-200/90">这个 &lt;{{ node.tagName }}&gt; 的 id 已被文档里更早的元素占用（section、stages、stage、quiz、figure、equation、ref 共用一个 id 空间），未渲染。</p>
  </div>
</template>
