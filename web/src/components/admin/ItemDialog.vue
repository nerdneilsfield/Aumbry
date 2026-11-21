<script setup lang="ts">
import { ref, watch } from 'vue'
import { useMutation, useQuery } from '@tanstack/vue-query'
import { useDateFormat } from '@vueuse/core'
import { ExternalLink, Trash2 } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import { itemPath } from '@/lib/url'
import { useAdmin } from '@/composables/useAdmin'
import Modal from '../Modal.vue'
import DirTree from './DirTree.vue'

const props = defineProps<{ itemId: string }>()
const emit = defineEmits<{ close: [] }>()

const { tree, invalidate } = useAdmin()
const { data: item, error: loadError } = useQuery({ queryKey: ['admin', 'item', props.itemId], queryFn: () => api.item(props.itemId) })
const mounts = ref<string[]>([])
watch(item, (i) => i && (mounts.value = i.dirs.map((d) => d.id)), { immediate: true })
const updatedAt = useDateFormat(() => item.value?.updated_at, 'YYYY-MM-DD HH:mm')

const saveMounts = useMutation({
  mutationKey: ['admin', 'mounts'],
  mutationFn: () => api.admin.setMounts(props.itemId, mounts.value),
  onSuccess: () => invalidate(),
})
const remove = useMutation({
  mutationKey: ['admin', 'delete-item'],
  mutationFn: () => api.admin.deleteItem(props.itemId),
  onSuccess: async () => {
    await invalidate()
    emit('close')
  },
})

const confirmRemove = () => confirm(`确定删除「${item.value?.title}」？此操作不可恢复。`) && remove.mutate()
const error = () => saveMounts.error.value ?? remove.error.value ?? loadError.value
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
    </div>

    <p v-if="error()" class="mt-4 text-sm text-red-400">{{ errorMessage(error()) }}</p>
    <div v-if="item" class="mt-6 flex justify-between gap-2">
      <button class="btn btn-danger" :disabled="remove.isPending.value" @click="confirmRemove"><Trash2 class="size-4" />删除条目</button>
      <button class="btn btn-primary" :disabled="saveMounts.isPending.value" @click="saveMounts.mutate()">保存挂载</button>
    </div>
  </Modal>
</template>
