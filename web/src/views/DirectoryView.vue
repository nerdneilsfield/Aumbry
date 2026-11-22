<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { keepPreviousData, useInfiniteQuery, useQuery } from '@tanstack/vue-query'
import { useTitle } from '@vueuse/core'
import { ChevronRight, FileQuestion, Folder, Lock, Search, X } from 'lucide-vue-next'
import { ApiError, api, errorMessage } from '@/api/client'
import ItemList from '@/components/ItemList.vue'
import PasswordGate from '@/components/PasswordGate.vue'
import SelectBox from '@/components/SelectBox.vue'
import StatePage from '@/components/StatePage.vue'
import { useDirFilters } from '@/composables/useDirFilters'
import { useSearchShortcut } from '@/composables/useSearchShortcut'

const route = useRoute()
const id = computed(() => route.params.id as string)

const dirQuery = useQuery({ queryKey: ['dir', id], queryFn: () => api.dir(id.value) })
const dir = dirQuery.data
const err = computed(() => (dirQuery.error.value instanceof ApiError ? dirQuery.error.value : undefined))
useTitle(computed(() => (dir.value ? `${dir.value.name} · Aumbry` : 'Aumbry')))

const { q, tag, type, active: filtering, settled, params, setType, clearTag } = useDirFilters()
const searchInput = ref<HTMLInputElement>()
useSearchShortcut(searchInput)

const typeOptions = [
  { value: 'all', label: '全部类型' },
  { value: 'report', label: '报告' },
  { value: 'post', label: '精读' },
]
const typeModel = computed({
  get: () => type.value || 'all',
  set: (v: string) => setType(v === 'report' || v === 'post' ? v : ''),
})

const searchQuery = useInfiniteQuery({
  queryKey: ['dir-search', id, params],
  queryFn: ({ pageParam }) => api.searchDir(id.value, params.value, 20, pageParam),
  initialPageParam: 0,
  getNextPageParam: (last) => (last.results.length >= last.limit ? last.offset + last.limit : undefined),
  // Text, tag and type all narrow the same listing, so any of them (even with an empty text) runs a search. The debounced
  // text lags the box: only search once it has caught up, so a stale term never hits another directory. Wait for the
  // directory itself first, so a locked one shows its password gate instead of a failing search.
  enabled: () => !!dir.value && filtering.value && settled.value,
  placeholderData: keepPreviousData,
})
const results = computed(() => searchQuery.data.value?.pages.flatMap((p) => p.results) ?? [])
const searchBusy = computed(() => !settled.value || searchQuery.isFetching.value)

// A search 401 means the unlock session lapsed: re-check the directory, which shows the gate.
watch(searchQuery.error, (e) => e instanceof ApiError && e.status === 401 && dirQuery.refetch())
</script>

<template>
  <PasswordGate v-if="err?.status === 401 && err.body.locked" :dir-id="id" :dir-name="err.body.dir_name" @unlocked="dirQuery.refetch()" />
  <StatePage v-else-if="err?.status === 404" :icon="FileQuestion" title="目录不存在" message="该目录可能已被删除，或链接有误。" />
  <StatePage v-else-if="err" :icon="FileQuestion" title="加载失败" :message="errorMessage(err)">
    <button class="btn" @click="dirQuery.refetch()">重试</button>
  </StatePage>
  <p v-else-if="!dir" class="py-20 text-center text-gray-500">加载中…</p>

  <div v-else class="mx-auto max-w-4xl">
    <nav class="mb-4 flex flex-wrap items-center gap-1 text-sm text-gray-400">
      <template v-for="(b, i) in dir.breadcrumb" :key="b.id">
        <ChevronRight v-if="i" class="size-4 text-gray-600" />
        <span v-if="b.id === dir.id" class="text-gray-200">{{ b.name }}</span>
        <RouterLink v-else :to="`/d/${encodeURIComponent(b.id)}`" class="hover:text-blue-300">{{ b.name }}</RouterLink>
      </template>
    </nav>

    <h1 class="text-2xl font-bold text-gray-50">{{ dir.name }}</h1>
    <p v-if="dir.description" class="mt-1 text-gray-400">{{ dir.description }}</p>

    <div class="relative mt-6">
      <Search class="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-gray-500" />
      <input ref="searchInput" v-model="q" type="search" class="input pl-9" placeholder="在此目录中搜索…（按 / 聚焦）" aria-label="在此目录中搜索" aria-keyshortcuts="/" />
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-3">
      <div class="w-36"><SelectBox v-model="typeModel" :options="typeOptions" label="按类型筛选" /></div>
      <button v-if="tag" type="button" class="chip inline-flex cursor-pointer items-center gap-1 bg-blue-500/20 py-1 text-sm text-blue-200 outline-none hover:bg-blue-500/30 focus-visible:ring-2 focus-visible:ring-blue-400" :aria-label="`清除标签筛选：${tag}`" @click="clearTag()">
        标签：{{ tag }}<X class="size-3.5" />
      </button>
    </div>

    <template v-if="filtering">
      <p v-if="searchQuery.error.value" class="mt-4 text-sm text-red-400">{{ errorMessage(searchQuery.error.value) }}</p>
      <p v-else-if="!results.length && searchBusy" class="py-10 text-center text-gray-500">搜索中…</p>
      <p v-else-if="!results.length" class="py-10 text-center text-gray-500">没有找到匹配的内容</p>
      <ItemList v-else class="mt-6" :items="results" :dir-id="id" :active-tag="tag" />
      <button v-if="searchQuery.hasNextPage.value" class="btn mx-auto mt-4" :disabled="searchQuery.isFetchingNextPage.value" @click="searchQuery.fetchNextPage()">加载更多</button>
    </template>

    <template v-else>
      <ul v-if="dir.children.length" class="mt-6 grid gap-3 sm:grid-cols-2">
        <li v-for="c in dir.children" :key="c.id">
          <RouterLink :to="`/d/${encodeURIComponent(c.id)}`" class="card flex h-full items-start gap-3 p-4 transition hover:border-gray-500 hover:bg-gray-700/50">
            <component :is="c.locked ? Lock : Folder" :class="['mt-0.5 size-5 shrink-0', c.locked ? 'text-amber-300' : 'text-blue-300']" />
            <div class="min-w-0">
              <h2 class="font-medium text-gray-50">{{ c.name }}</h2>
              <p v-if="c.description" class="mt-0.5 line-clamp-2 text-sm text-gray-400">{{ c.description }}</p>
            </div>
          </RouterLink>
        </li>
      </ul>

      <ItemList v-if="dir.items.length" class="mt-6" :items="dir.items" :dir-id="id" />
      <p v-else-if="!dir.children.length" class="py-10 text-center text-gray-500">这个目录还是空的</p>
    </template>
  </div>
</template>
