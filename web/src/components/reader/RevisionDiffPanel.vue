<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { ArrowRight, GitCompare } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import type { ItemRevisionSummary } from '@/api/types'
import { diffXmlDocuments } from '@/lib/revisionDiff'
import Modal from '../Modal.vue'
import SelectBox from '../SelectBox.vue'

const props = defineProps<{
  itemId: string
  revisions: ItemRevisionSummary[]
  initialLeftRev: number
  initialRightRev: number
  activeRev?: number | null
}>()

const emit = defineEmits<{ close: []; jump: [anchorId: string] }>()

const leftRevStr = ref(String(props.initialLeftRev))
const rightRevStr = ref(String(props.initialRightRev))

watch(
  () => [props.initialLeftRev, props.initialRightRev],
  ([l, r]) => {
    leftRevStr.value = String(l)
    rightRevStr.value = String(r)
  },
)

const leftRev = computed(() => Number(leftRevStr.value))
const rightRev = computed(() => Number(rightRevStr.value))

const options = computed(() =>
  props.revisions.map((r) => ({
    value: String(r.rev),
    label: `v${r.rev} · ${r.date} · ${r.title}`,
  })),
)

const leftQuery = useQuery({
  queryKey: ['item-rev-content', () => props.itemId, leftRev],
  queryFn: () => api.itemRevisionContent(props.itemId, leftRev.value),
  enabled: () => Number.isInteger(leftRev.value) && leftRev.value > 0,
})

const rightQuery = useQuery({
  queryKey: ['item-rev-content', () => props.itemId, rightRev],
  queryFn: () => api.itemRevisionContent(props.itemId, rightRev.value),
  enabled: () => Number.isInteger(rightRev.value) && rightRev.value > 0,
})

const loading = computed(() => leftQuery.isPending.value || rightQuery.isPending.value)
const err = computed(() => leftQuery.error.value ?? rightQuery.error.value)
const result = computed(() => {
  if (!leftQuery.data.value || !rightQuery.data.value) return null
  return diffXmlDocuments(leftQuery.data.value, rightQuery.data.value)
})

const kindMeta = {
  added: { label: '新增', badge: 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30' },
  removed: { label: '删除', badge: 'bg-red-500/15 text-red-300 border-red-500/30' },
  modified: { label: '修改', badge: 'bg-amber-500/15 text-amber-300 border-amber-500/30' },
} as const

function canJump(anchorId?: string) {
  return !!anchorId && (props.activeRev ?? null) === rightRev.value
}
</script>

<template>
  <Modal title="版本差异对比" wide @close="emit('close')">
    <div class="space-y-4">
      <div class="grid gap-3 sm:grid-cols-[1fr_auto_1fr] sm:items-end">
        <div>
          <label class="label">基准版本（旧）</label>
          <SelectBox v-model="leftRevStr" :options="options" label="选择基准版本" />
        </div>
        <ArrowRight class="hidden size-4 self-center text-gray-500 sm:mt-4 sm:block" aria-hidden="true" />
        <div>
          <label class="label">对比版本（新）</label>
          <SelectBox v-model="rightRevStr" :options="options" label="选择对比版本" />
        </div>
      </div>

      <p v-if="loading" class="py-8 text-center text-sm text-gray-400">正在拉取两版快照进行对比…</p>
      <p v-else-if="err" class="rounded-lg border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-300">
        {{ errorMessage(err) }}
      </p>

      <template v-else-if="result">
        <div class="flex flex-wrap items-center gap-2 rounded-lg border border-gray-700 bg-gray-900/70 px-3 py-2 text-xs text-gray-300">
          <GitCompare class="size-4 text-blue-300" aria-hidden="true" />
          <span>元数据变更 {{ result.stats.metaChanged }}</span>
          <span>·</span>
          <span class="text-emerald-300">新增块 {{ result.stats.added }}</span>
          <span>·</span>
          <span class="text-amber-300">修改块 {{ result.stats.modified }}</span>
          <span>·</span>
          <span class="text-red-300">删除块 {{ result.stats.removed }}</span>
        </div>

        <p v-if="!result.hasChanges" class="py-8 text-center text-sm text-gray-400">所选两个版本内容完全一致。</p>

        <div v-else class="space-y-4">
          <section v-if="result.metaChanges.length" class="space-y-2">
            <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-400">元数据变更</h3>
            <div v-for="mc in result.metaChanges" :key="mc.field" class="rounded-lg border border-gray-700 bg-gray-900/60 p-3 text-sm">
              <div class="mb-1 text-xs font-medium text-gray-400">{{ mc.label }}</div>
              <div v-if="mc.field === 'tags'" class="flex flex-wrap gap-1.5">
                <span
                  v-for="(p, idx) in mc.parts"
                  :key="idx"
                  :class="[
                    'chip',
                    p.added ? 'bg-emerald-500/20 text-emerald-300' : p.removed ? 'bg-red-500/20 text-red-300 line-through' : '',
                  ]"
                >
                  {{ p.added ? '+' : p.removed ? '-' : '' }}{{ p.value }}
                </span>
              </div>
              <p v-else class="whitespace-pre-wrap break-words text-gray-200">
                <template v-for="(p, idx) in mc.parts" :key="idx">
                  <ins v-if="p.added" class="rounded bg-emerald-500/20 px-0.5 text-emerald-300 no-underline">{{ p.value }}</ins>
                  <del v-else-if="p.removed" class="rounded bg-red-500/20 px-0.5 text-red-300 line-through">{{ p.value }}</del>
                  <span v-else>{{ p.value }}</span>
                </template>
              </p>
            </div>
          </section>

          <section v-if="result.blockChanges.length" class="space-y-2.5">
            <h3 class="text-xs font-semibold uppercase tracking-wide text-gray-400">正文块变更（{{ result.blockChanges.length }}）</h3>
            <div v-for="(ch, idx) in result.blockChanges" :key="idx" class="rounded-lg border border-gray-700 bg-gray-900/60 p-3 text-sm">
              <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
                <div class="flex min-w-0 items-center gap-2">
                  <span :class="['rounded border px-1.5 py-0.5 text-xs font-medium', kindMeta[ch.kind].badge]">
                    {{ kindMeta[ch.kind].label }}
                  </span>
                  <span class="truncate font-medium text-gray-200">{{ ch.label }}</span>
                </div>
                <button
                  v-if="canJump(ch.anchorId)"
                  type="button"
                  class="btn !px-2 !py-0.5 text-xs"
                  @click="ch.anchorId && (emit('jump', ch.anchorId), emit('close'))"
                >
                  跳到该块
                </button>
              </div>
              <pre class="max-h-56 overflow-y-auto whitespace-pre-wrap break-words font-sans text-xs leading-relaxed text-gray-300"><template v-for="(p, pIdx) in ch.parts" :key="pIdx"><ins v-if="p.added" class="rounded bg-emerald-500/20 px-0.5 text-emerald-300 no-underline">{{ p.value }}</ins><del v-else-if="p.removed" class="rounded bg-red-500/20 px-0.5 text-red-300 line-through">{{ p.value }}</del><span v-else>{{ p.value }}</span></template></pre>
            </div>
          </section>
        </div>
      </template>
    </div>
  </Modal>
</template>
