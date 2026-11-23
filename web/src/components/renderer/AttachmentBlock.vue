<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useTemplateRef, watch } from 'vue'
import type { PDFDocumentProxy, RenderTask } from 'pdfjs-dist'
import { ChevronLeft, ChevronRight, Download, FileText, ZoomIn, ZoomOut } from 'lucide-vue-next'
import { formatAttachmentExportMarkdown, pdfDocumentOptions } from '@/lib/pdf'
import { safeUrl } from '@/lib/url'
import { attr, safeId } from '@/lib/xml'
import { useAnchors } from '@/composables/useAnchors'

const props = defineProps<{ node: Element }>()
const anchors = useAnchors()

const canvasEl = useTemplateRef<HTMLCanvasElement>('canvasEl')

const src = computed(() => safeUrl(attr(props.node, 'src')))
const type = computed(() => attr(props.node, 'type'))
const title = computed(() => attr(props.node, 'title').trim())
const anchor = computed(() => (safeId(attr(props.node, 'id')) ? `att-${attr(props.node, 'id')}` : undefined))
const exportMd = computed(() => formatAttachmentExportMarkdown(title.value, attr(props.node, 'src')))

const totalPages = ref(0)
const currentPage = ref(1)
const pageInput = ref('1')
const scale = ref(1.25)
const loading = ref(false)
const loadError = ref('')

let pdfDoc: PDFDocumentProxy | null = null
let activeRenderTask: RenderTask | null = null
let loadSerial = 0

function cancelRender() {
  if (activeRenderTask) {
    try {
      activeRenderTask.cancel()
    } catch {
      /* ignore */
    }
    activeRenderTask = null
  }
}

async function destroyDoc() {
  cancelRender()
  if (pdfDoc) {
    try {
      await pdfDoc.destroy()
    } catch {
      /* ignore */
    }
    pdfDoc = null
  }
}

async function renderPage(num: number) {
  const doc = pdfDoc
  const canvas = canvasEl.value
  if (!doc || !canvas || num < 1 || num > totalPages.value) return

  cancelRender()
  loading.value = true
  try {
    const page = await doc.getPage(num)
    if (pdfDoc !== doc || currentPage.value !== num) return

    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const viewport = page.getViewport({ scale: scale.value })
    const outputScale = dpr

    canvas.width = Math.max(1, Math.floor(viewport.width * outputScale))
    canvas.height = Math.max(1, Math.floor(viewport.height * outputScale))
    canvas.style.width = `${Math.floor(viewport.width)}px`
    canvas.style.height = `${Math.floor(viewport.height)}px`

    const transform = outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : undefined
    const task = page.render({ canvas, viewport, transform })
    activeRenderTask = task
    await task.promise
    if (activeRenderTask === task) activeRenderTask = null
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err)
    if (!msg.toLowerCase().includes('cancelled')) {
      loadError.value = `第 ${num} 页渲染失败：${msg}`
    }
  } finally {
    loading.value = false
  }
}

async function loadDocument(url: string | undefined) {
  const serial = ++loadSerial
  await destroyDoc()
  totalPages.value = 0
  loadError.value = ''

  if (!url || type.value !== 'pdf') {
    loadError.value = '无效的 PDF 附件地址或类型。'
    return
  }

  loading.value = true
  try {
    const [pdfjs, workerMod] = await Promise.all([
      import('pdfjs-dist'),
      import('pdfjs-dist/build/pdf.worker.min.mjs?url'),
    ])
    if (serial !== loadSerial) return

    pdfjs.GlobalWorkerOptions.workerSrc = workerMod.default
    const loadingTask = pdfjs.getDocument(pdfDocumentOptions(url))
    const doc = await loadingTask.promise
    if (serial !== loadSerial) {
      await doc.destroy()
      return
    }
    pdfDoc = doc
    totalPages.value = doc.numPages

    const requested = anchors.attachmentPages.get(props.node)?.page ?? currentPage.value
    const clamped = Math.min(Math.max(1, requested), Math.max(1, doc.numPages))
    currentPage.value = clamped
    pageInput.value = String(clamped)
    await nextTick()
    await renderPage(clamped)
  } catch (err: unknown) {
    if (serial !== loadSerial) return
    const msg = err instanceof Error ? err.message : String(err)
    loadError.value = `无法加载 PDF：${msg}`
    loading.value = false
  }
}

function goToPage(raw: number) {
  if (!totalPages.value) return
  const clamped = Math.min(Math.max(1, Math.trunc(raw)), totalPages.value)
  currentPage.value = clamped
  pageInput.value = String(clamped)
  void renderPage(clamped)
}

function submitPageInput() {
  const parsed = Number.parseInt(pageInput.value.trim(), 10)
  if (!Number.isFinite(parsed)) {
    pageInput.value = String(currentPage.value)
    return
  }
  goToPage(parsed)
}

function adjustZoom(delta: number) {
  const next = Math.round(Math.min(2.5, Math.max(0.6, scale.value + delta)) * 100) / 100
  if (next === scale.value) return
  scale.value = next
  void renderPage(currentPage.value)
}

watch(
  () => anchors.attachmentPages.get(props.node),
  (req) => {
    if (!req) return
    if (totalPages.value > 0) {
      goToPage(req.page)
    } else {
      currentPage.value = req.page
      pageInput.value = String(req.page)
    }
  },
  { flush: 'post' },
)

watch(src, (nextUrl) => {
  void loadDocument(nextUrl)
})

onMounted(() => {
  void loadDocument(src.value)
})

onBeforeUnmount(() => {
  loadSerial++
  void destroyDoc()
})
</script>

<template>
  <section :id="anchor" :data-export-md="exportMd" class="not-prose card scroll-mt-20 space-y-3 p-4">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex min-w-0 items-center gap-2">
        <FileText class="size-4 shrink-0 text-amber-300" aria-hidden="true" />
        <h3 class="truncate text-base font-semibold text-gray-50">{{ title || '未命名 PDF 附件' }}</h3>
        <span v-if="totalPages" class="shrink-0 rounded bg-gray-800 px-2 py-0.5 text-xs text-gray-300">
          {{ totalPages }} 页
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-1.5 print:hidden">
        <template v-if="totalPages">
          <button
            type="button"
            class="btn !px-2 !py-1 text-xs"
            :disabled="currentPage <= 1 || loading"
            aria-label="上一页"
            @click="goToPage(currentPage - 1)"
          >
            <ChevronLeft class="size-3.5" aria-hidden="true" />
          </button>
          <form class="flex items-center gap-1 text-xs text-gray-300" @submit.prevent="submitPageInput">
            <input
              v-model="pageInput"
              type="text"
              inputmode="numeric"
              aria-label="页码"
              class="w-12 rounded border border-gray-700 bg-gray-900 px-1.5 py-0.5 text-center text-xs text-gray-100"
              @blur="submitPageInput"
            />
            <span>/ {{ totalPages }}</span>
          </form>
          <button
            type="button"
            class="btn !px-2 !py-1 text-xs"
            :disabled="currentPage >= totalPages || loading"
            aria-label="下一页"
            @click="goToPage(currentPage + 1)"
          >
            <ChevronRight class="size-3.5" aria-hidden="true" />
          </button>

          <button
            type="button"
            class="btn !px-2 !py-1 text-xs"
            :disabled="loading"
            aria-label="缩小"
            @click="adjustZoom(-0.2)"
          >
            <ZoomOut class="size-3.5" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="btn !px-2 !py-1 text-xs"
            :disabled="loading"
            aria-label="放大"
            @click="adjustZoom(0.2)"
          >
            <ZoomIn class="size-3.5" aria-hidden="true" />
          </button>
        </template>

        <a
          v-if="src"
          :href="src"
          :download="title ? `${title}.pdf` : 'attachment.pdf'"
          class="btn !px-2.5 !py-1 text-xs"
        >
          <Download class="size-3.5" aria-hidden="true" />下载 PDF
        </a>
      </div>
    </div>

    <p v-if="loadError" class="rounded border border-red-800/60 bg-red-950/30 px-3 py-2 text-sm text-red-300">
      {{ loadError }}
    </p>
    <div v-else class="relative overflow-x-auto rounded-lg border border-gray-700/80 bg-gray-950/60 p-3 text-center print:hidden">
      <canvas ref="canvasEl" class="mx-auto block max-w-full rounded shadow" />
      <div
        v-if="loading"
        class="mt-2 text-xs text-gray-400"
      >
        正在按需加载第 {{ currentPage }} 页…
      </div>
    </div>
    <p class="hidden text-sm text-gray-400 print:block">
      [PDF 附件] {{ title || '未命名 PDF 附件' }}
    </p>
  </section>
</template>
