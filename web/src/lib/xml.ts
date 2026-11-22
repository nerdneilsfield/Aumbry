import { inlineText } from '@/lib/markdown'
import { MAX_SECTION_DEPTH, anchorMap, safeId, sectionDepth, sectionId, stageDomId } from '@/lib/anchor'

export { MAX_SECTION_DEPTH, safeId, sectionDepth, sectionId, stageDomId }

export interface TocEntry {
  id: string
  title: string
  kind: 'section' | 'stage'
  /** indentation level: 1 + the number of <section> / <stage> ancestors, so a stage group sits under the section it is in */
  level: number
  /** id of the <stages> group the entry belongs to ('' = not inside a stage) and its 1-based stage index there */
  group: string
  stage: number
  /** first point of the entry's own <tldr>, plain text ('' when it has none) */
  summary: string
}

/** text of the first <point> of the <tldr> that is a direct child of `el` */
const firstPoint = (el: Element) => {
  const tldr = Array.from(el.children).find((c) => c.tagName === 'tldr')
  const p = tldr && Array.from(tldr.children).find((c) => c.tagName === 'point')
  return p ? inlineText(p.textContent?.trim() ?? '') : ''
}

/** Parses an Aumbry XML document; returns its <body> element and the TOC. Throws on malformed XML. */
export function parseXml(text: string) {
  const doc = new DOMParser().parseFromString(text, 'application/xml')
  const err = doc.querySelector('parsererror')
  if (err) throw new Error(`XML 解析失败：${err.textContent?.split('\n')[0] ?? ''}`)
  const body = doc.querySelector('document > body')
  if (!body) throw new Error('XML 缺少 <body>')
  return { body, toc: buildToc(body) }
}

/** TOC in document order: rendered stages and sections up to 3 levels. Unrendered subtrees are skipped. */
function buildToc(body: Element): TocEntry[] {
  const toc: TocEntry[] = []
  const nesting = (el: Element) => {
    let n = 1
    for (let p = el.parentElement; p; p = p.parentElement) if (p.tagName === 'section' || p.tagName === 'stage') n++
    return n
  }
  const map = anchorMap(body)
  for (const el of body.querySelectorAll('section, stage')) {
    if (map.isSkipped(el)) continue
    const own = el.parentElement?.closest('stage')
    const group = own?.parentElement?.getAttribute('id') ?? ''
    const stageEl = el.tagName === 'stage' ? el : own
    const index = stageEl ? Array.from(stageEl.parentElement?.children ?? []).filter((c) => c.tagName === 'stage').indexOf(stageEl) + 1 : 0
    if (el.tagName === 'stage') {
      // only stages of a well-formed group (a <stages> directly holding them) are listed
      if (el.parentElement?.tagName !== 'stages' || el.parentElement.closest('stage')) continue
      toc.push({ id: stageDomId(el), title: el.getAttribute('title') ?? '', kind: 'stage', level: nesting(el), group: el.parentElement.getAttribute('id') ?? '', stage: index, summary: firstPoint(el) })
      continue
    }
    if (sectionDepth(el) > MAX_SECTION_DEPTH) continue
    toc.push({ id: sectionId(el), title: el.getAttribute('title') ?? '', kind: 'section', level: nesting(el), group, stage: own ? index : 0, summary: firstPoint(el) })
  }
  return toc
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
  relatedIds: string[]
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
    relatedIds: Array.from(doc.querySelectorAll('document > meta > related-ids > id')).map((t) => t.textContent?.trim() ?? ''),
  }
}
