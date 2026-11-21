<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { useIntersectionObserver, useTitle } from '@vueuse/core'
import { Bot, CalendarDays, ChevronRight, ExternalLink, FileQuestion, FolderOpen, ShieldOff } from 'lucide-vue-next'
import { ApiError, api, errorMessage } from '@/api/client'
import { parseXml } from '@/lib/xml'
import { httpUrl } from '@/lib/url'
import ItemList from '@/components/ItemList.vue'
import StatePage from '@/components/StatePage.vue'
import XmlRenderer from '@/components/renderer/XmlRenderer.vue'

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

// Scroll-spy: the active TOC entry is the last section whose heading crossed the top of the viewport.
const active = shallowRef('')
const sections = shallowRef<HTMLElement[]>([])
// Re-run when the renderer mounts too: the content query may resolve before the item query.
watch([toc, rendered], ([t]) => {
  sections.value = t.flatMap((e) => document.getElementById(e.id) ?? [])
  active.value = t[0]?.id ?? ''
}, { flush: 'post', immediate: true })
useIntersectionObserver(sections, (entries) => entries.forEach((e) => e.isIntersecting && (active.value = e.target.id)), {
  rootMargin: '0px 0px -75% 0px',
})

const jump = (sectionId: string) => document.getElementById(sectionId)?.scrollIntoView({ behavior: 'smooth' })
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
    <header class="card mb-8 p-6">
      <nav v-if="item.dirs.length" class="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-gray-400">
        <FolderOpen class="size-4" />
        <RouterLink v-for="d in item.dirs" :key="d.id" :to="`/d/${encodeURIComponent(d.id)}`" class="inline-flex items-center hover:text-blue-300">
          {{ d.name }}<ChevronRight class="size-3.5 text-gray-600" />
        </RouterLink>
      </nav>
      <h1 class="text-2xl font-bold leading-tight text-gray-50 sm:text-3xl">{{ item.title }}</h1>
      <p v-if="item.summary" class="mt-3 text-gray-400">{{ item.summary }}</p>
      <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-sm text-gray-400">
        <span class="inline-flex items-center gap-1.5"><CalendarDays class="size-4" />{{ item.date }}</span>
        <span v-if="item.source_agent" class="inline-flex items-center gap-1.5"><Bot class="size-4" />{{ item.source_agent }}</span>
        <span v-for="t in item.tags" :key="t" class="chip">{{ t }}</span>
        <a v-if="httpUrl(item.canonical_url)" :href="httpUrl(item.canonical_url)" target="_blank" rel="noopener noreferrer" class="btn ml-auto">
          <ExternalLink class="size-4" />原文链接
        </a>
      </div>
    </header>

    <div class="grid gap-10 lg:grid-cols-[minmax(0,1fr)_14rem]">
      <article class="min-w-0">
        <p v-if="doc && 'error' in doc" class="card border-red-500/40 p-4 text-sm text-red-300">{{ doc.error }}</p>
        <XmlRenderer v-else-if="doc" :key="id" :body="doc.body" />
      </article>

      <aside v-if="toc.length" class="hidden lg:block">
        <nav class="sticky top-6 text-sm">
          <h2 class="mb-3 text-xs font-semibold uppercase tracking-wider text-gray-500">目录</h2>
          <ul class="space-y-1 border-l border-gray-700">
            <li v-for="t in toc" :key="t.id">
              <a
                :href="`#${t.id}`"
                :class="['-ml-px block border-l-2 py-1 pl-3 transition', active === t.id ? 'border-blue-400 text-blue-300' : 'border-transparent text-gray-400 hover:text-gray-100']"
                @click.prevent="jump(t.id)"
              >{{ t.title }}</a>
            </li>
          </ul>
        </nav>
      </aside>
    </div>

    <section v-if="item.related.length" class="mt-14 border-t border-gray-700 pt-8">
      <h2 class="mb-4 text-lg font-semibold text-gray-50">相关条目</h2>
      <ItemList :items="item.related" />
    </section>
  </div>
</template>
