<script setup lang="ts">
import { computed, ref, useTemplateRef } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { useDateFormat, useTitle } from '@vueuse/core'
import { AlertTriangle, ChevronRight, FileQuestion, FolderOpen, GitCompare, History, RotateCcw, ShieldOff } from 'lucide-vue-next'
import { ApiError, api, errorMessage } from '@/api/client'
import { readerBridge } from '@/composables/useReading'
import { useScrollSpy } from '@/composables/useScrollSpy'
import { parseMeta, parseXml } from '@/lib/xml'
import ItemList from '@/components/ItemList.vue'
import StatePage from '@/components/StatePage.vue'
import XmlRenderer from '@/components/renderer/XmlRenderer.vue'
import DocumentActions from '@/components/reader/DocumentActions.vue'
import MetaBar from '@/components/reader/MetaBar.vue'
import NeighborNav from '@/components/reader/NeighborNav.vue'
import RevisionDiffPanel from '@/components/reader/RevisionDiffPanel.vue'
import TocPanel from '@/components/reader/TocPanel.vue'

const route = useRoute()
const router = useRouter()
const id = computed(() => route.params.id as string)

// Metadata and XML are fetched in parallel; both share the same 403/404 semantics.
// ?dir= preserves directory context; ?rev=N selects a read-only historical revision snapshot.
const fromDir = computed(() => {
  const d = route.query.dir
  return typeof d === 'string' ? d : undefined
})
const rawRev = computed(() => {
  const r = route.query.rev
  return typeof r === 'string' ? r.trim() : Array.isArray(r) && typeof r[0] === 'string' ? r[0].trim() : ''
})
const revValid = computed(() => /^[1-9]\d*$/.test(rawRev.value))
const requestedRev = computed(() => (revValid.value ? Number(rawRev.value) : null))
const isRevisionMode = computed(() => rawRev.value !== '')

const metaQuery = useQuery({ queryKey: ['item', id, fromDir], queryFn: () => api.item(id.value, fromDir.value) })
const item = metaQuery.data

const revisionsQuery = useQuery({
  queryKey: ['item-revisions', id],
  queryFn: () => api.itemRevisions(id.value),
  enabled: () => !!item.value && (item.value.revision_count > 0 || isRevisionMode.value),
})
const revisions = computed(() => revisionsQuery.data.value?.revisions ?? [])

const contentQuery = useQuery({
  queryKey: ['item-content', id, requestedRev],
  queryFn: () => (requestedRev.value !== null ? api.itemRevisionContent(id.value, requestedRev.value) : api.itemContent(id.value)),
  enabled: () => !isRevisionMode.value || revValid.value,
})

const err = computed(() => metaQuery.error.value ?? (!isRevisionMode.value ? contentQuery.error.value : undefined))
const status = computed(() => (err.value instanceof ApiError ? err.value.status : 0))

const revErrorStatus = computed(() => (contentQuery.error.value instanceof ApiError ? contentQuery.error.value.status : 0))
const revErrorMsg = computed(() => {
  if (!isRevisionMode.value) return ''
  if (!revValid.value) return `历史版本号 "${rawRev.value}" 无效（必须为正整数）。`
  if (!contentQuery.error.value) return ''
  if (revErrorStatus.value === 403) return '你无权查看该历史版本（该快照仅曾发布于受保护目录）。'
  if (revErrorStatus.value === 404) return `第 ${requestedRev.value} 版历史快照不存在、已被管理员撤回，或超出保留上限被淘汰。`
  return errorMessage(contentQuery.error.value)
})

const activeRevisionMeta = computed(() => {
  if (requestedRev.value === null) return undefined
  return revisions.value.find((r) => r.rev === requestedRev.value)
})
const revCreatedAt = useDateFormat(() => activeRevisionMeta.value?.created_at, 'YYYY-MM-DD HH:mm')

const parsedRevMeta = computed(() => {
  if (!isRevisionMode.value || !contentQuery.data.value) return null
  return parseMeta(contentQuery.data.value)
})

const displayMeta = computed(() => {
  const it = item.value
  if (!it) return null
  if (parsedRevMeta.value) {
    return {
      title: parsedRevMeta.value.title || it.title,
      summary: parsedRevMeta.value.summary,
      date: parsedRevMeta.value.date || it.date,
      sourceAgent: parsedRevMeta.value.sourceAgent || it.source_agent,
      tags: parsedRevMeta.value.tags,
      canonicalUrl: parsedRevMeta.value.canonicalUrl,
    }
  }
  return {
    title: it.title,
    summary: it.summary,
    date: it.date,
    sourceAgent: it.source_agent,
    tags: it.tags,
    canonicalUrl: it.canonical_url,
  }
})

useTitle(
  computed(() =>
    displayMeta.value
      ? isRevisionMode.value && requestedRev.value !== null
        ? `${displayMeta.value.title} (v${requestedRev.value}) · Aumbry`
        : `${displayMeta.value.title} · Aumbry`
      : 'Aumbry',
  ),
)

const doc = computed(() => {
  if (!contentQuery.data.value) return
  try {
    return parseXml(contentQuery.data.value)
  } catch (e) {
    return { error: errorMessage(e) }
  }
})
const toc = computed(() => (doc.value && 'toc' in doc.value ? doc.value.toc : []))
const rendered = computed(() => (!err.value && !revErrorMsg.value && item.value && doc.value && !('error' in doc.value) ? doc.value : undefined))

const { active, jump } = useScrollSpy(toc, rendered)
const article = useTemplateRef<HTMLElement>('article')

const diffOpen = ref(false)
const diffPair = computed(() => {
  const list = revisions.value
  if (list.length < 2) return null
  const targetRev = requestedRev.value ?? list[0].rev
  const idx = list.findIndex((r) => r.rev === targetRev)
  if (idx >= 0 && idx + 1 < list.length) {
    return { leftRev: list[idx + 1].rev, rightRev: list[idx].rev }
  }
  if (idx > 0) {
    return { leftRev: list[idx].rev, rightRev: list[idx - 1].rev }
  }
  return { leftRev: list[1].rev, rightRev: list[0].rev }
})

function openLatest() {
  const nextQuery: Record<string, string> = {}
  if (fromDir.value) nextQuery.dir = fromDir.value
  void router.push({ path: route.path, query: nextQuery })
}

function openRevision(rev: number) {
  if (!Number.isInteger(rev) || rev < 1) return
  const nextQuery: Record<string, string> = { rev: String(rev) }
  if (fromDir.value) nextQuery.dir = fromDir.value
  void router.push({ path: route.path, query: nextQuery })
}

function onSelectRevision(e: Event) {
  const val = Number((e.target as HTMLSelectElement).value)
  if (Number.isInteger(val) && val > 0) openRevision(val)
}

function onDiffJump(anchorId: string) {
  void readerBridge.value?.reveal(`#${anchorId}`)
}
</script>

<template>
  <StatePage v-if="status === 403" :icon="ShieldOff" title="无权访问" message="这篇内容受密码保护。请通过分享给你的目录链接进入并输入密码。">
    <RouterLink to="/" class="btn">返回首页</RouterLink>
  </StatePage>
  <StatePage v-else-if="status === 404" :icon="FileQuestion" title="内容不存在" message="该内容可能已被删除，或链接有误。">
    <RouterLink to="/" class="btn">返回首页</RouterLink>
  </StatePage>
  <StatePage v-else-if="err" :icon="FileQuestion" title="加载失败" :message="errorMessage(err)">
    <button class="btn" @click="metaQuery.refetch(); contentQuery.refetch()">重试</button>
  </StatePage>
  <p v-else-if="!item" class="py-20 text-center text-gray-500">加载中…</p>

  <div v-else>
    <div
      v-if="isRevisionMode && !revErrorMsg"
      class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-200 print:hidden"
      data-revision-banner
    >
      <div class="flex min-w-0 items-center gap-2">
        <History class="size-4 shrink-0 text-amber-300" aria-hidden="true" />
        <span>
          正在查看第 <strong>{{ requestedRev }}</strong> 版历史快照<template v-if="activeRevisionMeta">（发布于 {{ revCreatedAt }}）</template>，内容只读（不计入阅读进度）
        </span>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <select
          v-if="revisions.length > 0"
          :value="String(requestedRev)"
          aria-label="切换历史版本"
          class="rounded-lg border border-amber-500/40 bg-gray-900 px-2.5 py-1 text-xs text-gray-100 outline-none focus:border-blue-400"
          @change="onSelectRevision"
        >
          <option v-for="r in revisions" :key="r.rev" :value="String(r.rev)">v{{ r.rev }} · {{ r.date }}</option>
        </select>
        <button v-if="diffPair" type="button" class="btn !py-1 text-xs" @click="diffOpen = true">
          <GitCompare class="size-3.5" aria-hidden="true" />查看与上一版差异
        </button>
        <button type="button" class="btn btn-primary !py-1 text-xs" @click="openLatest()">
          <RotateCcw class="size-3.5" aria-hidden="true" />返回最新版
        </button>
      </div>
    </div>

    <div
      v-else-if="isRevisionMode && revErrorMsg"
      class="card mb-6 space-y-3 border-amber-500/40 p-5 text-sm print:hidden"
      data-revision-error
    >
      <div class="flex items-center gap-2 font-semibold text-amber-300">
        <AlertTriangle class="size-4 shrink-0" aria-hidden="true" />无法加载指定历史快照
      </div>
      <p class="text-gray-300">{{ revErrorMsg }}</p>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-primary" @click="openLatest()">
          <RotateCcw class="size-4" aria-hidden="true" />返回最新版
        </button>
        <RouterLink to="/" class="btn">返回首页</RouterLink>
      </div>
    </div>

    <template v-if="!(isRevisionMode && revErrorMsg) && displayMeta">
      <MetaBar
        :title="displayMeta.title"
        :summary="displayMeta.summary"
        :date="displayMeta.date"
        :source-agent="displayMeta.sourceAgent"
        :tags="displayMeta.tags"
        :canonical-url="displayMeta.canonicalUrl"
        :tag-dir-id="item.neighbors?.dir.id"
      >
        <template #actions>
          <div v-if="!isRevisionMode && item.revision_count > 0" class="flex flex-wrap items-center gap-2 print:hidden">
            <select
              v-if="revisions.length > 0"
              value=""
              aria-label="历史版本"
              class="rounded-lg border border-gray-600 bg-gray-900 px-2.5 py-1.5 text-xs text-gray-200 outline-none focus:border-blue-400"
              @change="onSelectRevision"
            >
              <option value="" disabled>历史版本（{{ item.revision_count }}）</option>
              <option v-for="r in revisions" :key="r.rev" :value="String(r.rev)">
                v{{ r.rev }}{{ r.is_current ? '（当前）' : '' }} · {{ r.date }}
              </option>
            </select>
            <button v-if="diffPair" type="button" class="btn !py-1.5 text-xs" @click="diffOpen = true">
              <GitCompare class="size-3.5" aria-hidden="true" />版本对比
            </button>
          </div>
          <DocumentActions
            v-if="rendered"
            :content="article"
            :meta="displayMeta"
          />
        </template>
        <template v-if="item.dirs.length" #crumbs>
          <nav class="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-gray-400">
            <FolderOpen class="size-4" />
            <RouterLink v-for="d in item.dirs" :key="d.id" :to="`/d/${encodeURIComponent(d.id)}`" class="inline-flex items-center hover:text-blue-300">
              {{ d.name }}<ChevronRight class="size-3.5 text-gray-600" />
            </RouterLink>
          </nav>
        </template>
      </MetaBar>

      <p v-if="contentQuery.isPending.value" class="py-16 text-center text-gray-500">加载快照正文…</p>
      <div v-else class="grid gap-10 lg:grid-cols-[minmax(0,1fr)_14rem] print:block">
        <article ref="article" class="min-w-0">
          <p v-if="doc && 'error' in doc" class="card border-red-500/40 p-4 text-sm text-red-300">{{ doc.error }}</p>
          <XmlRenderer v-else-if="doc" :key="`${id}:${requestedRev ?? 'latest'}`" :body="doc.body" :storage-key="id" :read-only="isRevisionMode" />
        </article>

        <TocPanel :toc="toc" :active="active" @jump="jump" />
      </div>
    </template>

    <NeighborNav v-if="item.neighbors && !isRevisionMode" :neighbors="item.neighbors" />

    <section v-if="item.referenced_by.length" class="mt-14 border-t border-gray-700 pt-8">
      <h2 class="mb-4 text-lg font-semibold text-gray-50">收录于 / 被以下简报引用</h2>
      <ItemList :items="item.referenced_by" />
    </section>

    <section v-if="item.related.length" class="mt-14 border-t border-gray-700 pt-8">
      <h2 class="mb-4 text-lg font-semibold text-gray-50">相关条目</h2>
      <ItemList :items="item.related" />
    </section>

    <RevisionDiffPanel
      v-if="diffOpen && diffPair && revisions.length >= 2"
      :item-id="id"
      :revisions="revisions"
      :initial-left-rev="diffPair.leftRev"
      :initial-right-rev="diffPair.rightRev"
      :active-rev="requestedRev ?? item.current_rev"
      @close="diffOpen = false"
      @jump="onDiffJump"
    />
  </div>
</template>
