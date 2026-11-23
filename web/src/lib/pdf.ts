export interface PdfDocumentInitOptions {
  url: string
  withCredentials: boolean
  disableAutoFetch: boolean
  disableStream: boolean
  isEvalSupported: boolean
}

/**
 * Returns hardened pdfjs-dist `getDocument` options per PRD §7.2:
 * - disables background whole-file prefetching (`disableAutoFetch: true`)
 * - disables streaming mode so pdfjs requests Byte-Range chunks on demand (`disableStream: true`)
 * - disables PDF JS/eval support (`isEvalSupported: false`)
 */
export function pdfDocumentOptions(url: string): PdfDocumentInitOptions {
  return {
    url,
    withCredentials: true,
    disableAutoFetch: true,
    disableStream: true,
    isEvalSupported: false,
  }
}

/** Formats an offline/Markdown export placeholder for a PDF `<attachment>` block without bitmaps. */
export function formatAttachmentExportMarkdown(title: string, src?: string): string {
  const cleanTitle = title.trim() || 'PDF 附件'
  const cleanSrc = src?.trim()
  return cleanSrc ? `> PDF 附件：${cleanTitle}（${cleanSrc}）` : `> PDF 附件：${cleanTitle}`
}
