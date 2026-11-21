<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useTitle } from '@vueuse/core'
import { FlaskConical, Library } from 'lucide-vue-next'
import { parseMeta, parseXml } from '@/lib/xml'
import XmlRenderer from '@/components/renderer/XmlRenderer.vue'
import MetaBar from '@/components/reader/MetaBar.vue'
import TocPanel from '@/components/reader/TocPanel.vue'
import ThemeToggle from '@/components/ThemeToggle.vue'
import { useScrollSpy } from '@/composables/useScrollSpy'
import { FIXTURES, rewriteImages } from './fixtures'

// The chosen fixture lives in ?doc=… (replaceState only, works from file://) so a reload keeps it.
const initial = new URLSearchParams(location.search).get('doc')
const current = ref(FIXTURES.find((f) => f.name === initial)?.name ?? FIXTURES[0].name)
watch(current, (name) => {
  const u = new URL(location.href)
  u.searchParams.set('doc', name)
  u.hash = ''
  history.replaceState(history.state, '', u)
  window.scrollTo({ top: 0 })
})

const fixture = computed(() => FIXTURES.find((f) => f.name === current.value)!)
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
useTitle(computed(() => `${meta.value.title} · Aumbry v1.1 预览`))
const { active, jump } = useScrollSpy(toc, doc)
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

    <nav class="-mx-4 mt-4 overflow-x-auto px-4 sm:mx-0 sm:px-0" aria-label="示例文档">
      <ul class="m-0 flex list-none gap-2 p-0">
        <li v-for="f in FIXTURES" :key="f.name" class="shrink-0">
          <button
            type="button"
            :aria-pressed="f.name === current"
            :title="f.hint"
            :data-fixture="f.name"
            :class="[
              'cursor-pointer rounded-lg border px-3.5 py-2 text-sm font-medium transition',
              f.name === current ? 'border-blue-400 bg-blue-500/15 text-blue-300' : 'border-gray-600 text-gray-300 hover:bg-gray-700',
            ]"
            @click="current = f.name"
          >{{ f.label }}</button>
        </li>
      </ul>
      <p class="mt-2 text-xs text-gray-500">{{ fixture.hint }}</p>
    </nav>

    <main class="py-8">
      <MetaBar :title="meta.title" :summary="meta.summary" :date="meta.date" :source-agent="meta.sourceAgent" :tags="meta.tags" :canonical-url="meta.canonicalUrl" />
      <div class="grid gap-10 lg:grid-cols-[minmax(0,1fr)_14rem]">
        <article class="min-w-0">
          <p v-if="'error' in doc" class="card border-red-500/40 p-4 text-sm text-red-300">{{ doc.error }}</p>
          <XmlRenderer v-else :key="fixture.name" :body="doc.body" />
        </article>
        <TocPanel :toc="toc" :active="active" @jump="jump" />
      </div>
    </main>
  </div>
</template>
