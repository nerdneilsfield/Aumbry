<script setup lang="ts">
import { ref } from 'vue'
import { onKeyStroke } from '@vueuse/core'
import { FileDown, Printer } from 'lucide-vue-next'
import { printDocument, withExpandedDocument } from '@/composables/usePrint'
import type { ExportMeta } from '@/lib/exportMarkdown'

// Print and "export Markdown" (PRD 9.3). Both work on the whole document, not on what the reader has unlocked or opened.
const props = defineProps<{ content: HTMLElement | null | undefined; meta: ExportMeta }>()

const exporting = ref(false)
async function exportMarkdown() {
  const article = props.content
  if (!article || exporting.value) return
  exporting.value = true
  try {
    // turndown is loaded on demand; the page is expanded only for the moment the DOM is read
    const { toMarkdown, downloadText, markdownFilename } = await import('@/lib/exportMarkdown')
    const markdown = await withExpandedDocument(() => toMarkdown(article, props.meta))
    downloadText(markdownFilename(props.meta.title), markdown)
  } finally {
    exporting.value = false
  }
}

// Ctrl/Cmd+P goes through printDocument() too, so lazily drawn diagrams and charts are ready when the dialog opens.
onKeyStroke('p', (e) => {
  if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey) {
    e.preventDefault()
    void printDocument()
  }
})
</script>

<template>
  <div class="flex items-center gap-2 print:hidden" data-document-actions>
    <button type="button" class="btn" data-action="print" title="打印：展开全部内容，白底黑字" @click="printDocument()"><Printer class="size-4" aria-hidden="true" />打印</button>
    <button type="button" class="btn" data-action="export-markdown" title="导出为 Markdown 文件（包含全部阶段与折叠内容）" :disabled="exporting" @click="exportMarkdown()">
      <FileDown class="size-4" aria-hidden="true" />导出 Markdown
    </button>
  </div>
</template>
