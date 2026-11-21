<script setup lang="ts">
import { computed, ref } from 'vue'
import { keepPreviousData, useInfiniteQuery, useMutation } from '@tanstack/vue-query'
import { refDebounced, useClipboard } from '@vueuse/core'
import { FolderInput, FolderPlus, Inbox, KeyRound, Layers, Link2, LogOut, Pencil, Search, Trash2 } from 'lucide-vue-next'
import { api, errorMessage } from '@/api/client'
import type { AdminSearchParams } from '@/api/types'
import { findNode } from '@/lib/tree'
import { useAdmin } from '@/composables/useAdmin'
import DirDialog from '@/components/admin/DirDialog.vue'
import DirTree from '@/components/admin/DirTree.vue'
import ItemDialog from '@/components/admin/ItemDialog.vue'
import LoginForm from '@/components/admin/LoginForm.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ItemList from '@/components/ItemList.vue'
import SelectBox from '@/components/SelectBox.vue'

const { session, tree, unmountedCount, logout, invalidate } = useAdmin()

// Filters: the sidebar picks a directory or the "unmounted" bucket; the bar adds text/type/date.
const selected = ref<string[]>([])
const selectedId = computed(() => selected.value[0] ?? null)
const unmounted = ref(false)
const q = ref('')
const type = ref('all')
const from = ref('')
const to = ref('')
const dq = refDebounced(q, 300)

const dialog = ref<{ mode: 'create' | 'edit' | 'password' | 'move'; dirId?: string; parentId?: string | null }>()
const itemId = ref<string>()

const selectedDir = computed(() => findNode(tree.value, selectedId.value))
const dialogDir = computed(() => findNode(tree.value, dialog.value?.dirId ?? null))
const typeOptions = [
  { value: 'all', label: '全部类型' },
  { value: 'report', label: '报告' },
  { value: 'post', label: '精读' },
]

const params = computed(() => ({
  q: dq.value.trim(),
  type: (type.value === 'all' ? '' : type.value) as AdminSearchParams['type'],
  from: from.value,
  to: to.value,
  dir: selectedId.value,
  unmounted: unmounted.value,
}))
const list = useInfiniteQuery({
  queryKey: ['admin', 'search', params],
  queryFn: ({ pageParam }) => api.admin.search(params.value, 20, pageParam),
  initialPageParam: 0,
  getNextPageParam: (last) => (last.results.length >= last.limit ? last.offset + last.limit : undefined),
  enabled: () => session.value === 'in',
  placeholderData: keepPreviousData,
})
const results = computed(() => list.data.value?.pages.flatMap((p) => p.results) ?? [])

function pick(dirId: string | null, onlyUnmounted = false) {
  selected.value = dirId ? [dirId] : []
  unmounted.value = onlyUnmounted
}

const { copy, copied } = useClipboard({ legacy: true })
const copyShare = (id: string) => copy(`${location.origin}/d/${id}`)

const removeDir = useMutation({
  mutationKey: ['admin', 'delete-dir'],
  mutationFn: (id: string) => api.admin.deleteDir(id),
  onSuccess: () => {
    selected.value = []
    return invalidate()
  },
})
const dirToRemove = ref<{ id: string; name: string }>()
const doRemoveDir = () => dirToRemove.value && removeDir.mutate(dirToRemove.value.id, { onSettled: () => (dirToRemove.value = undefined) })
</script>

<template>
  <p v-if="session === 'unknown'" class="py-20 text-center text-gray-500">加载中…</p>
  <LoginForm v-else-if="session === 'out'" />

  <div v-else>
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-2xl font-bold text-gray-50">管理控制台</h1>
      <button class="btn" @click="logout.mutate()"><LogOut class="size-4" />退出登录</button>
    </div>

    <div class="grid items-start gap-6 lg:grid-cols-[18rem_minmax(0,1fr)]">
      <aside class="card space-y-1 p-3 lg:sticky lg:top-6">
        <button :class="['flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm', !selectedId && !unmounted ? 'bg-blue-500/15 text-blue-200' : 'text-gray-300 hover:bg-gray-700/60']" @click="pick(null)">
          <Layers class="size-4" />全部条目
        </button>
        <button :class="['flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm', unmounted ? 'bg-blue-500/15 text-blue-200' : 'text-gray-300 hover:bg-gray-700/60']" @click="pick(null, true)">
          <Inbox class="size-4" />未挂载条目<span class="ml-auto text-xs text-gray-500">{{ unmountedCount }}</span>
        </button>
        <div class="my-2 border-t border-gray-700" />
        <DirTree v-if="tree.length" :model-value="selected" :items="tree" @update:model-value="pick($event[0] ?? null)" />
        <p v-else class="px-2 py-2 text-sm text-gray-500">还没有目录</p>
        <button class="btn mt-2 w-full" @click="dialog = { mode: 'create', parentId: null }"><FolderPlus class="size-4" />新建根目录</button>
      </aside>

      <section class="min-w-0 space-y-5">
        <div class="card grid gap-3 p-4 sm:grid-cols-[1fr_auto_auto_auto]">
          <div class="relative sm:col-span-4">
            <Search class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-gray-500" />
            <input v-model="q" type="search" class="input pl-9" placeholder="全站搜索（留空则列出最新条目）" />
          </div>
          <SelectBox v-model="type" :options="typeOptions" label="类型" />
          <input v-model="from" type="date" class="input" aria-label="起始日期" />
          <input v-model="to" type="date" class="input" aria-label="截止日期" />
          <label class="flex items-center gap-2 text-sm text-gray-300">
            <input v-model="unmounted" type="checkbox" class="accent-blue-500" />仅未挂载
          </label>
        </div>

        <div v-if="selectedDir" class="card p-4">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="min-w-0">
              <h2 class="truncate text-lg font-semibold text-gray-50">{{ selectedDir.name }}</h2>
              <p v-if="selectedDir.description" class="truncate text-sm text-gray-400">{{ selectedDir.description }}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <button class="btn" @click="copyShare(selectedDir.id)"><Link2 class="size-4" />{{ copied ? '已复制' : '复制分享链接' }}</button>
              <button class="btn" @click="dialog = { mode: 'create', parentId: selectedDir.id }"><FolderPlus class="size-4" />子目录</button>
              <button class="btn" @click="dialog = { mode: 'edit', dirId: selectedDir.id }"><Pencil class="size-4" />编辑</button>
              <button class="btn" @click="dialog = { mode: 'password', dirId: selectedDir.id }"><KeyRound class="size-4" />密码</button>
              <button class="btn" @click="dialog = { mode: 'move', dirId: selectedDir.id }"><FolderInput class="size-4" />移动</button>
              <button class="btn btn-danger" @click="dirToRemove = { id: selectedDir.id, name: selectedDir.name }"><Trash2 class="size-4" />删除</button>
            </div>
          </div>
        </div>

        <p v-if="list.error.value || removeDir.error.value" class="text-sm text-red-400">{{ errorMessage(list.error.value ?? removeDir.error.value) }}</p>

        <p v-if="list.isPending.value" class="py-10 text-center text-gray-500">加载中…</p>
        <p v-else-if="!results.length" class="py-10 text-center text-gray-500">没有符合条件的条目</p>
        <ItemList v-else :items="results" selectable @select="itemId = $event.id" />
        <button v-if="list.hasNextPage.value" class="btn mx-auto" :disabled="list.isFetchingNextPage.value" @click="list.fetchNextPage()">加载更多</button>
      </section>
    </div>

    <DirDialog v-if="dialog" :mode="dialog.mode" :dir="dialogDir" :parent-id="dialog.parentId" @close="dialog = undefined" />
    <ConfirmDialog
      :open="!!dirToRemove"
      title="删除目录"
      :description="`确定删除目录「${dirToRemove?.name}」及其所有子目录？其中的条目不会被删除，但会失去这些挂载。`"
      :pending="removeDir.isPending.value"
      @update:open="dirToRemove = undefined"
      @confirm="doRemoveDir"
    />
    <ItemDialog v-if="itemId" :item-id="itemId" @close="itemId = undefined" />
  </div>
</template>
