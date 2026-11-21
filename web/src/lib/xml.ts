export interface TocEntry {
  id: string
  title: string
}

/** Stable anchor id: position of the section among all sections of its document. */
export const sectionId = (el: Element) => `sec-${Array.from(el.ownerDocument.querySelectorAll<Element>('section')).indexOf(el)}`

/** Parses an Aumbry XML document; returns its <body> element and the TOC. Throws on malformed XML. */
export function parseXml(text: string) {
  const doc = new DOMParser().parseFromString(text, 'application/xml')
  const err = doc.querySelector('parsererror')
  if (err) throw new Error(`XML 解析失败：${err.textContent?.split('\n')[0] ?? ''}`)
  const body = doc.querySelector('document > body')
  if (!body) throw new Error('XML 缺少 <body>')
  const toc: TocEntry[] = Array.from(body.querySelectorAll('section')).map((s) => ({
    id: sectionId(s),
    title: s.getAttribute('title') ?? '',
  }))
  return { body, toc }
}

export const children = (el: Element) => Array.from(el.children)
export const attr = (el: Element, name: string) => el.getAttribute(name) ?? ''

export interface DocMeta {
  title: string
  summary: string
  date: string
  sourceAgent: string
  canonicalUrl: string
  tags: string[]
}

/** Reads the <meta> block of an Aumbry XML document (the offline demo has no API to supply it). */
export function parseMeta(text: string): DocMeta {
  const doc = new DOMParser().parseFromString(text, 'application/xml')
  const m = (name: string) => doc.querySelector(`document > meta > ${name}`)?.textContent?.trim() ?? ''
  return {
    title: m('title'),
    summary: m('summary'),
    date: m('date'),
    sourceAgent: m('source-agent'),
    canonicalUrl: m('canonical-url'),
    tags: Array.from(doc.querySelectorAll('document > meta > tags > tag')).map((t) => t.textContent?.trim() ?? ''),
  }
}

/** `id` values are limited to [a-z0-9-]{1,32} by the protocol; anything else never becomes a DOM id. */
export const safeId = (raw: string) => (/^[a-z0-9-]{1,32}$/.test(raw) ? raw : '')
