<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { useDateFormat } from '@vueuse/core'
import { ExternalLink, History, Trash2 } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import { itemPath } from '@/lib/url'
import { useAdmin } from '@/composables/useAdmin'
import ConfirmDialog from '../ConfirmDialog.vue'
import Modal from '../Modal.vue'
import DirTree from './DirTree.vue'

const props = defineProps<{ itemId: string }>()
const emit = defineEmits<{ close: [] }>()

const qc = useQueryClient()
const { tree, invalidate } = useAdmin()
const { data: item, error: loadError } = useQuery({ queryKey: ['admin', 'item', props.itemId], queryFn: () => api.item(props.itemId) })
const { data: revData, error: revError } = useQuery({ queryKey: ['admin', 'item-revisions', props.itemId], queryFn: () => api.itemRevisions(props.itemId) })
const mounts = ref<string[]>([])
watch(item, (i) => i && (mounts.value = i.dirs.map((d) => d.id)), { immediate: true })
const updatedAt = useDateFormat(() => item.value?.updated_at, 'YYYY-MM-DD HH:mm')

const saveMounts = useMutation({
  mutationKey: ['admin', 'mounts'],
  mutationFn: () => api.admin.setMounts(props.itemId, mounts.value),
  onSuccess: async () => {
    await Promise.all([
      invalidate(),
      qc.invalidateQueries({ queryKey: ['admin', 'item', props.itemId] }),
      qc.invalidateQueries({ queryKey: ['admin', 'item-revisions', props.itemId] }),
    ])
  },
})
const remove = useMutation({
  mutationKey: ['admin', 'delete-item'],
  mutationFn: () => api.admin.deleteItem(props.itemId),
  onSuccess: async () => {
    await invalidate()
    emit('close')
  },
})
const removeRev = useMutation({
  mutationKey: ['admin', 'delete-revision'],
  mutationFn: (rev: number) => api.admin.deleteRevision(props.itemId, rev),
  onSuccess: async () => {
    revToDelete.value = undefined
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['admin', 'item', props.itemId] }),
      qc.invalidateQueries({ queryKey: ['admin', 'item-revisions', props.itemId] }),
    ])
  },
})

const confirming = ref(false)
const revToDelete = ref<number>()
const doRemove = () => remove.mutate(undefined, { onSettled: () => (confirming.value = false) })
const doRemoveRev = () => revToDelete.value && removeRev.mutate(revToDelete.value)
function fmtRevTime(iso: string) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
const error = () => saveMounts.error.value ?? remove.error.value ?? removeRev.error.value ?? loadError.value ?? revError.value
</script>

<template>
  <Modal :title="item?.title ?? '条目详情'" wide @close="emit('close')">
    <p v-if="!item && !loadError" class="text-gray-500">加载中…</p>
    <div v-if="item" class="space-y-5">
      <div class="space-y-2 text-sm text-gray-400">
        <p v-if="item.summary" class="text-gray-300">{{ item.summary }}</p>
        <p>
          {{ item.type === 'post' ? '精读' : '报告' }} · {{ item.date }}<template v-if="item.source_agent"> · {{ item.source_agent }}</template>
        </p>
        <p v-if="item.tags.length" class="flex flex-wrap gap-1.5"><span v-for="t in item.tags" :key="t" class="chip">{{ t }}</span></p>
        <p class="text-xs text-gray-500">ID：{{ item.id }} · 更新于 {{ updatedAt }}</p>
        <a :href="itemPath(item)" target="_blank" rel="noopener" class="btn mt-1"><ExternalLink class="size-4" />打开阅读页</a>
      </div>

      <div>
        <h3 class="label">挂载目录（可多选）</h3>
        <div class="max-h-64 overflow-y-auto rounded-lg border border-gray-700 bg-gray-900 p-2">
          <DirTree v-model="mounts" :items="tree" multiple />
          <p v-if="!tree.length" class="p-2 text-sm text-gray-500">还没有目录</p>
        </div>
        <p v-if="!mounts.length" class="mt-1.5 text-xs text-amber-300">未选择任何目录：该条目将处于「未挂载」状态，访客无法访问。</p>
      </div>

      <div v-if="revData && revData.revisions.length > 0">
        <h3 class="label flex items-center gap-1.5"><History class="size-3.5" />修订历史（{{ revData.revisions.length }} 个版本）</h3>
        <ul class="max-h-48 divide-y divide-gray-700 overflow-y-auto rounded-lg border border-gray-700 bg-gray-900">
          <li v-for="r in revData.revisions" :key="r.rev" class="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-xs">
            <div class="min-w-0">
              <span class="font-semibold text-gray-200">v{{ r.rev }}</span>
              <span v-if="r.is_current" class="ml-1.5 rounded bg-blue-500/20 px-1.5 py-0.5 text-[0.65rem] text-blue-300">当前</span>
              <span class="ml-2 text-gray-400">{{ fmtRevTime(r.created_at) }}</span>
              <span class="ml-2 truncate text-gray-500">{{ r.title }}</span>
            </div>
            <div class="flex items-center gap-1.5">
              <a :href="`${itemPath(item)}?rev=${r.rev}`" target="_blank" rel="noopener" class="btn !px-2 !py-0.5 text-xs">
                <ExternalLink class="size-3" />查看
              </a>
              <button
                type="button"
                class="btn btn-danger !px-2 !py-0.5 text-xs"
                :disabled="r.is_current"
                :title="r.is_current ? '当前最新版本不可单独删除' : `删除第 ${r.rev} 版快照`"
                @click="revToDelete = r.rev"
              >
                <Trash2 class="size-3" />删除
              </button>
            </div>
          </li>
        </ul>
      </div>
    </div>

    <p v-if="error()" class="mt-4 text-sm text-red-400">{{ errorMessage(error()) }}</p>
    <div v-if="item" class="mt-6 flex justify-between gap-2">
      <button class="btn btn-danger" :disabled="remove.isPending.value" @click="confirming = true"><Trash2 class="size-4" />删除条目</button>
      <button class="btn btn-primary" :disabled="saveMounts.isPending.value" @click="saveMounts.mutate()">保存挂载</button>
    </div>
    <ConfirmDialog
      v-model:open="confirming"
      title="删除条目"
      :description="`确定删除「${item?.title}」？此操作不可恢复。`"
      :pending="remove.isPending.value"
      @confirm="doRemove"
    />
    <ConfirmDialog
      :open="revToDelete !== undefined"
      title="删除历史快照"
      :description="`确定撤回并删除「${item?.title}」的第 ${revToDelete} 版历史快照？`"
      :pending="removeRev.isPending.value"
      @update:open="(o) => !o && (revToDelete = undefined)"
      @confirm="doRemoveRev"
    />
  </Modal>
</template>
