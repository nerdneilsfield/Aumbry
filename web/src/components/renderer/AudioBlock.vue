<script setup lang="ts">
import { computed, ref, useTemplateRef } from 'vue'
import { CollapsibleContent, CollapsibleRoot, CollapsibleTrigger } from 'reka-ui'
import { ChevronRight, Compass, FileText, Headphones, ListMusic } from 'lucide-vue-next'
import { activeChapterIndex, formatAudioExportMarkdown, parseAudioTimestamp } from '@/lib/audio'
import { safeUrl } from '@/lib/url'
import { attr, children, safeId } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'
import { useExpanded } from '@/composables/usePrint'

const props = defineProps<{ node: Element }>()
const { canTarget, navigate } = useAnchors()
const expanded = useExpanded()

const audioEl = useTemplateRef<HTMLAudioElement>('audioEl')
const currentTime = ref(0)
const transcriptOpen = ref(false)

const src = computed(() => safeUrl(attr(props.node, 'src')))
const title = computed(() => attr(props.node, 'title').trim())
const anchor = computed(() => (safeId(attr(props.node, 'id')) ? `audio-${attr(props.node, 'id')}` : undefined))

const chapters = computed(() =>
  children(props.node)
    .filter((c) => c.tagName === 'chapter')
    .map((c) => {
      const start = attr(c, 'start').trim()
      const startSeconds = parseAudioTimestamp(start) ?? 0
      const target = attr(c, 'target')
      return {
        start,
        startSeconds,
        title: attr(c, 'title').trim(),
        target,
        targetValid: target ? canTarget(target) : false,
      }
    }),
)

const transcript = computed(() => {
  const el = children(props.node).find((c) => c.tagName === 'transcript')
  return el?.textContent?.trim() ?? ''
})

const activeIdx = computed(() => activeChapterIndex(chapters.value, currentTime.value))
const exportMd = computed(() => formatAudioExportMarkdown(title.value, chapters.value, transcript.value))

function onTimeUpdate() {
  if (audioEl.value) currentTime.value = audioEl.value.currentTime
}

function seekTo(seconds: number) {
  const el = audioEl.value
  if (!el) return
  el.currentTime = seconds
  currentTime.value = seconds
  void el.play().catch(() => {})
}
</script>

<template>
  <section :id="anchor" :data-export-md="exportMd" class="not-prose card scroll-mt-20 space-y-3 p-4">
    <div class="flex items-center gap-2.5">
      <Headphones class="size-4 shrink-0 text-blue-300" aria-hidden="true" />
      <h3 class="text-base font-semibold text-gray-50">{{ title || '未命名音频' }}</h3>
    </div>

    <audio
      v-if="src"
      ref="audioEl"
      controls
      preload="metadata"
      :src="src"
      class="w-full print:hidden"
      @timeupdate="onTimeUpdate"
      @seeked="onTimeUpdate"
    />
    <p v-else class="text-sm text-red-300">音频地址无效或不可用。</p>

    <div v-if="chapters.length" class="space-y-1.5 border-t border-gray-700 pt-3">
      <p class="flex items-center gap-1.5 text-xs font-medium text-gray-400">
        <ListMusic class="size-3.5" aria-hidden="true" />章节（{{ chapters.length }}）
      </p>
      <ul class="space-y-1">
        <li
          v-for="(ch, i) in chapters"
          :key="i"
          :class="[
            'flex flex-wrap items-center justify-between gap-2 rounded-lg px-2.5 py-1.5 text-sm transition',
            activeIdx === i ? 'bg-blue-500/15 text-blue-200' : 'text-gray-300 hover:bg-gray-700/40',
          ]"
        >
          <button
            type="button"
            class="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 text-left"
            @click="seekTo(ch.startSeconds)"
          >
            <code class="shrink-0 font-mono text-xs text-blue-300">{{ ch.start }}</code>
            <span class="min-w-0 break-words">{{ ch.title }}</span>
          </button>
          <button
            v-if="ch.target"
            type="button"
            class="btn !px-2 !py-1 text-xs print:hidden"
            :disabled="!ch.targetValid"
            :title="ch.targetValid ? `定位到 ${ch.target}` : 'INVALID_TARGET：目标锚点不存在'"
            @click.stop="ch.targetValid && navigate(ch.target)"
          >
            <Compass class="size-3.5" aria-hidden="true" />定位到正文
          </button>
        </li>
      </ul>
    </div>

    <CollapsibleRoot v-if="transcript" v-model:open="transcriptOpen" class="border-t border-gray-700 pt-3">
      <CollapsibleTrigger class="flex w-full cursor-pointer items-center gap-2 text-left text-sm font-medium text-gray-300 hover:text-gray-100 print:hidden">
        <ChevronRight :class="['size-4 shrink-0 text-gray-400 transition-transform', transcriptOpen && 'rotate-90']" />
        <FileText class="size-4 shrink-0 text-gray-400" aria-hidden="true" />
        逐字稿（Transcript）
      </CollapsibleTrigger>
      <CollapsibleContent :force-mount="expanded" class="mt-3 max-h-96 overflow-y-auto whitespace-pre-wrap rounded-lg border border-gray-700/80 bg-gray-900/70 p-3 text-sm leading-relaxed text-gray-300 print:block! print:max-h-none">
        {{ transcript }}
      </CollapsibleContent>
    </CollapsibleRoot>
  </section>
</template>
