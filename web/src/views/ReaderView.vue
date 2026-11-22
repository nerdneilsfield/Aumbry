<script setup lang="ts">
import { computed, useTemplateRef } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { useTitle } from '@vueuse/core'
import { ChevronRight, FileQuestion, FolderOpen, ShieldOff } from 'lucide-vue-next'
import { ApiError, api, errorMessage } from '@/api/client'
import { parseXml } from '@/lib/xml'
import ItemList from '@/components/ItemList.vue'
import StatePage from '@/components/StatePage.vue'
import XmlRenderer from '@/components/renderer/XmlRenderer.vue'
import MetaBar from '@/components/reader/MetaBar.vue'
import TocPanel from '@/components/reader/TocPanel.vue'
import DocumentActions from '@/components/reader/DocumentActions.vue'
import { useScrollSpy } from '@/composables/useScrollSpy'

const route = useRoute()
const id = computed(() => route.params.id as string)

// Metadata and XML are fetched in parallel; both share the same 403/404 semantics.
const metaQuery = useQuery({ queryKey: ['item', id], queryFn: () => api.item(id.value) })
const contentQuery = useQuery({ queryKey: ['item-content', id], queryFn: () => api.itemContent(id.value) })
const item = metaQuery.data
const err = computed(() => metaQuery.error.value ?? contentQuery.error.value)
const status = computed(() => (err.value instanceof ApiError ? err.value.status : 0))
useTitle(computed(() => (item.value ? `${item.value.title} · Aumbry` : 'Aumbry')))

const doc = computed(() => {
  if (!contentQuery.data.value) return
  try {
    return parseXml(contentQuery.data.value)
  } catch (e) {
    return { error: errorMessage(e) }
  }
})
const toc = computed(() => (doc.value && 'toc' in doc.value ? doc.value.toc : []))
// The parsed document once <XmlRenderer> is actually in the DOM (item loaded, no error, XML valid).
const rendered = computed(() => (!err.value && item.value && doc.value && !('error' in doc.value) ? doc.value : undefined))

const { active, jump } = useScrollSpy(toc, rendered)
const article = useTemplateRef<HTMLElement>('article')
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
    <MetaBar :title="item.title" :summary="item.summary" :date="item.date" :source-agent="item.source_agent" :tags="item.tags" :canonical-url="item.canonical_url">
      <template v-if="rendered" #actions>
        <DocumentActions
          :content="article"
          :meta="{ title: item.title, summary: item.summary, date: item.date, sourceAgent: item.source_agent, tags: item.tags, canonicalUrl: item.canonical_url }"
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

    <div class="grid gap-10 lg:grid-cols-[minmax(0,1fr)_14rem] print:block">
      <article ref="article" class="min-w-0">
        <p v-if="doc && 'error' in doc" class="card border-red-500/40 p-4 text-sm text-red-300">{{ doc.error }}</p>
        <XmlRenderer v-else-if="doc" :key="id" :body="doc.body" :storage-key="id" />
      </article>

      <TocPanel :toc="toc" :active="active" @jump="jump" />
    </div>

    <section v-if="item.related.length" class="mt-14 border-t border-gray-700 pt-8">
      <h2 class="mb-4 text-lg font-semibold text-gray-50">相关条目</h2>
      <ItemList :items="item.related" />
    </section>
  </div>
</template>
