<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMutation } from '@tanstack/vue-query'
import { api, errorMessage } from '@/api/client'
import type { DirNode } from '@/api/types'
import { findParentId, flattenTree } from '@/lib/tree'
import { useAdmin } from '@/composables/useAdmin'
import Modal from '../Modal.vue'
import SelectBox from '../SelectBox.vue'

type Mode = 'create' | 'edit' | 'password' | 'move'

const props = defineProps<{ mode: Mode; dir?: DirNode; parentId?: string | null }>()
const emit = defineEmits<{ close: [] }>()

const ROOT = '__root__' // Reka Select does not accept empty option values
const { tree, invalidate } = useAdmin()
const name = ref(props.mode === 'edit' ? (props.dir?.name ?? '') : '')
const description = ref(props.mode === 'edit' ? (props.dir?.description ?? '') : '')
const password = ref('')
const target = ref((props.dir && findParentId(tree.value, props.dir.id)) || ROOT)

const titles = { create: '新建目录', edit: '编辑目录', password: '目录密码', move: '移动目录' }
const parentOptions = computed(() => [
  { value: ROOT, label: '（根目录）' },
  ...flattenTree(tree.value, props.dir?.id).map((d) => ({ value: d.id, label: d.name, depth: d.depth + 1 })),
])

const save = useMutation({
  mutationKey: ['admin', 'dir'],
  mutationFn: async (clearPassword: boolean) => {
    const id = props.dir?.id ?? ''
    switch (props.mode) {
      case 'create':
        await api.admin.createDir({
          parent_id: props.parentId ?? null,
          name: name.value.trim(),
          description: description.value.trim(),
          ...(password.value ? { password: password.value } : {}),
        })
        break
      case 'edit':
        await api.admin.patchDir(id, { name: name.value.trim(), description: description.value.trim() })
        break
      case 'password':
        await api.admin.patchDir(id, { password: clearPassword ? '' : password.value })
        break
      case 'move':
        await api.admin.patchDir(id, { parent_id: target.value === ROOT ? null : target.value })
    }
  },
  onSuccess: async () => {
    await invalidate()
    emit('close')
  },
})
</script>

<template>
  <Modal :title="`${titles[mode]}${dir ? `：${dir.name}` : ''}`" @close="emit('close')">
    <form class="space-y-4" @submit.prevent="save.mutate(false)">
      <template v-if="mode === 'create' || mode === 'edit'">
        <div>
          <label class="label">名称</label>
          <input v-model="name" class="input" required autofocus />
        </div>
        <div>
          <label class="label">描述（可选）</label>
          <input v-model="description" class="input" />
        </div>
      </template>

      <div v-if="mode === 'create' || mode === 'password'">
        <label class="label">{{ mode === 'create' ? '访问密码（可选，留空则不设密码）' : '新密码' }}</label>
        <input v-model="password" type="password" class="input" autocomplete="new-password" :required="mode === 'password'" />
        <p v-if="mode === 'password'" class="mt-2 text-xs text-gray-500">
          {{ dir?.has_password ? '当前已设置密码。' : '当前未设置密码。' }}密码会同时保护其所有子目录。
        </p>
      </div>

      <div v-if="mode === 'move'">
        <label class="label">移动到</label>
        <SelectBox v-model="target" :options="parentOptions" label="目标目录" />
      </div>

      <p v-if="save.error.value" class="text-sm text-red-400">{{ errorMessage(save.error.value) }}</p>
      <div class="flex justify-end gap-2">
        <button v-if="mode === 'password' && dir?.has_password" type="button" class="btn btn-danger mr-auto" :disabled="save.isPending.value" @click="save.mutate(true)">清除密码</button>
        <button type="button" class="btn" @click="emit('close')">取消</button>
        <button class="btn btn-primary" :disabled="save.isPending.value">保存</button>
      </div>
    </form>
  </Modal>
</template>
