<script setup lang="ts">
import { computed, provide, ref, watch } from 'vue'
import { useTitle } from '@vueuse/core'
import { ArrowLeft, FlaskConical, Layers, Library, Link2, TriangleAlert } from 'lucide-vue-next'
import { parseMeta, parseXml } from '@/lib/xml'
import { validateDoc } from '@/lib/validate'
import { urlParams } from '@/composables/useUrlParams'
import { DETAIL_NAV } from '@/composables/useDetailNav'
import XmlRenderer from '@/components/renderer/XmlRenderer.vue'
import MetaBar from '@/components/reader/MetaBar.vue'
import TocPanel from '@/components/reader/TocPanel.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'
import { useScrollSpy } from '@/composables/useScrollSpy'
import { FIXTURES, KINDS, rewriteImages } from './fixtures'

// The chosen fixture lives in ?doc=… (VueUse useUrlSearchParams, replaceState only: works from file://) so a reload keeps it.
const current = ref(FIXTURES.find((f) => f.name === urlParams.doc)?.name ?? FIXTURES[0].name)
// Reading a detail / related entry pushes the page we came from here, so "返回" works.
const trail = ref<string[]>([])

watch(current, (name) => {
  // a #stage… / #fig… hash belongs to the document that was on screen
  history.replaceState(history.state, '', location.pathname + location.search)
  urlParams.doc = name
  window.scrollTo({ top: 0 })
})
const select = (name: string) => {
  trail.value = []
  current.value = name
}

const fixture = computed(() => FIXTURES.find((f) => f.name === current.value)!)
const groups = computed(() => KINDS.map((kind) => ({ kind, items: FIXTURES.filter((f) => f.kind === kind && !f.hidden) })).filter((g) => g.items.length))
const meta = computed(() => parseMeta(fixture.value.xml))
const doc = computed(() => {
  try {
    const d = parseXml(fixture.value.xml)
    rewriteImages(d.body)
    return d
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) }
  }
})
const toc = computed(() => ('toc' in doc.value ? doc.value.toc : []))
const issues = computed(() => ('body' in doc.value ? validateDoc(doc.value.body) : []))
useTitle(computed(() => `${meta.value.title} · Aumbry v1.1 预览`))
const { active, jump } = useScrollSpy(toc, doc)

// <news-item detail-id> and meta/related-ids: the offline stand-in for opening another item by id.
const notice = ref('')
let noticeTimer = 0
function openById(id: string) {
  const target = FIXTURES.find((f) => f.id === id)
  if (!target) {
    notice.value = `离线预览只带了 ${FIXTURES.length} 篇示例，没有 id 为 ${id} 的条目（真实站点会打开 /p/${id}）。`
    clearTimeout(noticeTimer)
    noticeTimer = window.setTimeout(() => (notice.value = ''), 6000)
    return
  }
  notice.value = ''
  trail.value.push(current.value)
  current.value = target.name
}
provide(DETAIL_NAV, openById)
const back = () => {
  const prev = trail.value.pop()
  if (prev) current.value = prev
}
const backLabel = computed(() => FIXTURES.find((f) => f.name === trail.value.at(-1))?.label ?? '')
const related = computed(() => meta.value.relatedIds.map((id) => ({ id, target: FIXTURES.find((f) => f.id === id) })))
</script>

<template>
  <div class="mx-auto w-full max-w-7xl px-4 sm:px-6">
    <header class="flex min-h-16 flex-wrap items-center gap-x-3 gap-y-2 border-b border-gray-700 py-3">
      <span class="inline-flex items-center gap-2 text-lg font-bold text-gray-50"><Library class="size-5 text-blue-300" />Aumbry</span>
      <span class="inline-flex items-center gap-1 rounded-full bg-violet-500/15 px-2.5 py-0.5 text-xs font-semibold text-violet-300 ring-1 ring-inset ring-violet-400/40">
        <FlaskConical class="size-3.5" />XML v1.1 富文本预览 · 离线
      </span>
      <ThemeToggle class="ml-auto" />
    </header>

    <nav class="mt-4 space-y-2" aria-label="示例文档" data-fixture-nav>
      <div v-for="g in groups" :key="g.kind" class="flex flex-wrap items-center gap-x-2 gap-y-1.5">
        <span class="w-16 shrink-0 text-xs font-semibold text-gray-500">{{ g.kind }}</span>
        <button
          v-for="f in g.items"
          :key="f.name"
          type="button"
          :aria-pressed="f.name === current"
          :title="f.hint"
          :data-fixture="f.name"
          :class="[
            'inline-flex cursor-pointer items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition',
            f.name === current ? 'border-blue-400 bg-blue-500/15 text-blue-300' : 'border-gray-600 text-gray-300 hover:bg-gray-700',
          ]"
          @click="select(f.name)"
        >
          {{ f.label }}
          <span v-if="f.staged" class="inline-flex items-center gap-0.5 rounded bg-violet-500/15 px-1.5 py-0.5 text-[0.65rem] font-semibold text-violet-300" data-staged-badge><Layers class="size-3" aria-hidden="true" />分阶段</span>
        </button>
      </div>
      <p class="pt-1 text-xs text-gray-500">{{ fixture.hint }}</p>
    </nav>

    <main class="py-8">
      <button v-if="trail.length" type="button" class="btn mb-4" data-action="back" @click="back"><ArrowLeft class="size-4" aria-hidden="true" />返回《{{ backLabel }}》</button>
      <p v-if="notice" class="card mb-4 border-amber-500/40 p-3 text-sm text-amber-300" role="status" data-notice>{{ notice }}</p>

      <MetaBar :title="meta.title" :summary="meta.summary" :date="meta.date" :source-agent="meta.sourceAgent" :tags="meta.tags" :canonical-url="meta.canonicalUrl" />

      <div v-if="related.length" class="-mt-4 mb-8 flex flex-wrap items-center gap-2 text-sm" data-related>
        <span class="inline-flex items-center gap-1 text-gray-400"><Link2 class="size-4" aria-hidden="true" />相关条目</span>
        <button v-for="r in related" :key="r.id" type="button" class="btn !py-1" :data-related-id="r.id" @click="openById(r.id)">{{ r.target?.label ?? r.id }}</button>
      </div>

      <details v-if="issues.length" class="card mb-8 border-amber-500/40 p-4 text-sm" data-issues>
        <summary class="flex cursor-pointer items-center gap-2 font-semibold text-amber-300">
          <TriangleAlert class="size-4 shrink-0" aria-hidden="true" />校验发现 {{ issues.length }} 个问题
          <span class="font-normal text-gray-400">（展开查看；每处在正文里都有对应的错误块，其余内容照常显示）</span>
        </summary>
        <ul class="m-0 mt-3 list-none space-y-1.5 p-0">
          <li v-for="(i, k) in issues" :key="k" class="flex flex-wrap gap-x-2 text-gray-300">
            <code class="rounded bg-amber-500/10 px-1.5 text-amber-300">{{ i.code }}</code>
            <span v-if="i.where" class="text-gray-400">{{ i.where }}</span>
            <span>{{ i.message }}</span>
          </li>
        </ul>
      </details>

      <div class="grid gap-10 lg:grid-cols-[minmax(0,1fr)_16rem]">
        <article class="min-w-0">
          <p v-if="'error' in doc" class="card border-red-500/40 p-4 text-sm text-red-300">{{ doc.error }}</p>
          <XmlRenderer v-else :key="fixture.name" :body="doc.body" :storage-key="fixture.storeAs ?? fixture.name" />
        </article>
        <TocPanel :toc="toc" :active="active" @jump="jump" />
      </div>
    </main>
  </div>
</template>
