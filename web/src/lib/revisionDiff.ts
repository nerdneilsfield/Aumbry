import { diffLines, diffWordsWithSpace } from 'diff'

export interface DiffMeta {
  title: string
  summary: string
  tags: string[]
}

export interface DiffBlock {
  matchKey: string
  tag: string
  id?: string
  anchorId?: string
  label: string
  text: string
}

export interface DiffDocument {
  meta: DiffMeta
  blocks: DiffBlock[]
}

export interface DiffPart {
  value: string
  added?: boolean
  removed?: boolean
}

export interface MetaFieldChange {
  field: 'title' | 'summary' | 'tags'
  label: string
  parts: DiffPart[]
}

export interface BlockChange {
  kind: 'added' | 'removed' | 'modified'
  tag: string
  id?: string
  anchorId?: string
  label: string
  parts: DiffPart[]
}

export interface RevisionDiffResult {
  hasChanges: boolean
  metaChanges: MetaFieldChange[]
  blockChanges: BlockChange[]
  stats: {
    added: number
    removed: number
    modified: number
    metaChanged: number
  }
}

interface XmlLiteNode {
  tag: string
  attrs: Record<string, string>
  children: XmlLiteNode[]
  text: string
}

const ANCHOR_PREFIX: Record<string, string> = {
  section: 's',
  stages: 'stages',
  stage: 'stage',
  quiz: 'quiz',
  figure: 'fig',
  equation: 'eq',
  ref: 'ref',
  audio: 'audio',
}

const LEAF_TAGS = new Set([
  'md',
  'figure',
  'figure-group',
  'audio',
  'paper-card',
  'repo-card',
  'compare-table',
  'callout',
  'sources',
  'claim',
  'equation',
  'mermaid',
  'chart',
  'metrics',
  'code-block',
  'quote',
  'checklist',
  'timeline',
  'glossary',
  'references',
  'quiz',
  'next-steps',
  'search-log',
  'tldr',
  'news-item',
])

const CONTAINER_TAGS = new Set(['body', 'section', 'stages', 'stage', 'collapse', 'tabs', 'tab', 'steps', 'step'])

function decodeXmlEntities(raw: string): string {
  return raw.replace(/&(#x[0-9a-fA-F]+|#\d+|amp|lt|gt|quot|apos);/g, (_, ent: string) => {
    switch (ent) {
      case 'amp':
        return '&'
      case 'lt':
        return '<'
      case 'gt':
        return '>'
      case 'quot':
        return '"'
      case 'apos':
        return "'"
      default:
        if (ent.startsWith('#x')) {
          const cp = Number.parseInt(ent.slice(2), 16)
          return Number.isFinite(cp) ? String.fromCodePoint(cp) : `&${ent};`
        }
        if (ent.startsWith('#')) {
          const cp = Number.parseInt(ent.slice(1), 10)
          return Number.isFinite(cp) ? String.fromCodePoint(cp) : `&${ent};`
        }
        return `&${ent};`
    }
  })
}

function parseAttributes(raw: string): Record<string, string> {
  const attrs: Record<string, string> = {}
  const re = /([A-Za-z_][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')/g
  for (const m of raw.matchAll(re)) {
    attrs[m[1]] = decodeXmlEntities(m[2] ?? m[3] ?? '')
  }
  return attrs
}

function elementToLite(el: Element): XmlLiteNode {
  const attrs: Record<string, string> = {}
  for (const a of Array.from(el.attributes)) {
    attrs[a.name] = a.value
  }
  const kids = Array.from(el.children)
  return {
    tag: el.tagName,
    attrs,
    children: kids.map(elementToLite),
    text: kids.length === 0 ? (el.textContent ?? '') : collectDirectText(el),
  }
}

function collectDirectText(el: Element): string {
  let out = ''
  for (const n of Array.from(el.childNodes)) {
    if (n.nodeType === 3 || n.nodeType === 4) out += n.textContent ?? ''
  }
  return out
}

/** Pure-TS XML parser fallback so `diffXmlDocuments` runs cleanly in Node unit tests without DOMParser. */
function parseXmlLiteFallback(xml: string): XmlLiteNode | null {
  const root: XmlLiteNode = { tag: '__root__', attrs: {}, children: [], text: '' }
  const stack: XmlLiteNode[] = [root]
  let i = 0
  const n = xml.length

  while (i < n) {
    if (xml.startsWith('<![CDATA[', i)) {
      const end = xml.indexOf(']]>', i + 9)
      const closeIdx = end === -1 ? n : end
      stack[stack.length - 1].text += xml.slice(i + 9, closeIdx)
      i = end === -1 ? n : end + 3
      continue
    }
    if (xml.startsWith('<!--', i)) {
      const end = xml.indexOf('-->', i + 4)
      i = end === -1 ? n : end + 3
      continue
    }
    if (xml.startsWith('<?', i)) {
      const end = xml.indexOf('?>', i + 2)
      i = end === -1 ? n : end + 2
      continue
    }
    if (xml.startsWith('<!', i)) {
      const end = xml.indexOf('>', i + 2)
      i = end === -1 ? n : end + 1
      continue
    }

    const lt = xml.indexOf('<', i)
    if (lt === -1) {
      stack[stack.length - 1].text += decodeXmlEntities(xml.slice(i))
      break
    }
    if (lt > i) {
      stack[stack.length - 1].text += decodeXmlEntities(xml.slice(i, lt))
      i = lt
    }

    if (xml[i + 1] === '/') {
      const gt = xml.indexOf('>', i + 2)
      if (gt === -1) break
      const closeTag = xml.slice(i + 2, gt).trim()
      for (let k = stack.length - 1; k >= 1; k--) {
        if (stack[k].tag === closeTag) {
          stack.length = k
          break
        }
      }
      i = gt + 1
      continue
    }

    // Scan start tag taking care of quotes inside attributes.
    let j = i + 1
    let quote: string | null = null
    while (j < n) {
      const ch = xml[j]
      if (quote) {
        if (ch === quote) quote = null
      } else if (ch === '"' || ch === "'") {
        quote = ch
      } else if (ch === '>') {
        break
      }
      j++
    }
    if (j >= n) break
    let inner = xml.slice(i + 1, j).trim()
    const selfClosing = inner.endsWith('/')
    if (selfClosing) inner = inner.slice(0, -1).trim()
    const wsIdx = inner.search(/\s/)
    const tag = wsIdx === -1 ? inner : inner.slice(0, wsIdx)
    const attrStr = wsIdx === -1 ? '' : inner.slice(wsIdx + 1)
    if (tag) {
      const node: XmlLiteNode = { tag, attrs: parseAttributes(attrStr), children: [], text: '' }
      stack[stack.length - 1].children.push(node)
      if (!selfClosing) stack.push(node)
    }
    i = j + 1
  }

  return root.children[0] ?? null
}

function parseRootLite(xml: string): XmlLiteNode | null {
  if (typeof DOMParser !== 'undefined') {
    const parsed = new DOMParser().parseFromString(xml, 'application/xml')
    if (!parsed.querySelector('parsererror') && parsed.documentElement) {
      return elementToLite(parsed.documentElement)
    }
  }
  return parseXmlLiteFallback(xml)
}

const firstChild = (node: XmlLiteNode, tag: string) => node.children.find((c) => c.tag === tag)
const allChildren = (node: XmlLiteNode, tag: string) => node.children.filter((c) => c.tag === tag)

function fullText(node: XmlLiteNode): string {
  if (node.children.length === 0) return node.text.trim()
  return [node.text.trim(), ...node.children.map(fullText)].filter(Boolean).join(' ').trim()
}

function serializeLeafBlock(node: XmlLiteNode): string {
  switch (node.tag) {
    case 'md':
    case 'equation':
    case 'mermaid':
    case 'chart':
    case 'code-block':
      return node.text.trim()
    case 'figure':
      return [node.attrs.caption, node.attrs.alt, node.attrs.src].filter(Boolean).join(' · ')
    case 'figure-group':
      return allChildren(node, 'figure')
        .map((f) => [f.attrs.caption, f.attrs.src].filter(Boolean).join(': '))
        .join('\n')
    case 'audio': {
      const head = `标题：${node.attrs.title ?? ''}`
      const srcLine = node.attrs.src ? `音频：${node.attrs.src}` : ''
      const chapters = allChildren(node, 'chapter').map(
        (c) => `${c.attrs.start ?? ''} ${c.attrs.title ?? ''}${c.attrs.target ? ` (${c.attrs.target})` : ''}`.trim(),
      )
      const transcript = firstChild(node, 'transcript')?.text.trim() ?? ''
      return [head, srcLine, ...chapters, transcript ? `逐字稿：\n${transcript}` : ''].filter(Boolean).join('\n')
    }
    case 'tldr': {
      const points = allChildren(node, 'point').map((p) => `- ${fullText(p)}`)
      const verdict = firstChild(node, 'verdict')
      if (verdict) points.push(`结论：${fullText(verdict)}`)
      return points.join('\n')
    }
    case 'quiz': {
      const q = firstChild(node, 'question')
      const opts = allChildren(node, 'option').map(
        (o) => `${o.attrs.correct === 'true' ? '[x]' : '[ ]'} ${fullText(o)}`,
      )
      const exp = firstChild(node, 'explain')
      return [q ? fullText(q) : '', ...opts, exp ? `解析：${fullText(exp)}` : ''].filter(Boolean).join('\n')
    }
    case 'compare-table': {
      const cols = firstChild(node, 'columns')
      const colNames = cols ? allChildren(cols, 'col').map(fullText) : []
      const rows = allChildren(node, 'row').map((r) =>
        allChildren(r, 'cell')
          .map((c) => (c.attrs.tag ? `[${c.attrs.tag}] ` : '') + fullText(c))
          .join(' | '),
      )
      return [colNames.join(' | '), ...rows].filter(Boolean).join('\n')
    }
    default: {
      const attrParts = Object.entries(node.attrs)
        .filter(([k]) => k !== 'id')
        .map(([k, v]) => `${k}=${v}`)
      const kidParts = node.children.map((c) => `${c.tag}: ${serializeLeafBlock(c)}`)
      return [attrParts.join(', '), node.text.trim(), ...kidParts].filter(Boolean).join('\n')
    }
  }
}

function blockLabel(node: XmlLiteNode, ordinal: number): string {
  const hint =
    node.attrs.title ||
    node.attrs.caption ||
    node.attrs.name ||
    node.attrs.label ||
    node.attrs.id ||
    ''
  const tagNames: Record<string, string> = {
    section: '章节',
    stage: '阶段',
    md: 'Markdown 段落',
    figure: '图片',
    'figure-group': '图片组',
    audio: '音频',
    quiz: '测验',
    equation: '公式',
    chart: '图表',
    mermaid: '流程图',
    'code-block': '代码块',
    'compare-table': '对比表',
    claim: '论断',
    metrics: '指标',
    callout: '提示框',
    tldr: '摘要 (TL;DR)',
    'news-item': '资讯条目',
    references: '参考文献',
    'next-steps': '下一步',
    checklist: '检查清单',
    timeline: '时间线',
    glossary: '术语表',
    quote: '引文',
    sources: '来源列表',
    'search-log': '检索记录',
    'paper-card': '论文卡片',
    'repo-card': '仓库卡片',
  }
  const base = tagNames[node.tag] ?? node.tag
  if (hint) return `${base}：${hint}`
  return `${base} #${ordinal}`
}

/** Parses an Aumbry XML string into structured metadata and ordered blocks for revision comparison. */
export function parseXmlToDiffDocument(xml: string): DiffDocument {
  const docEl = parseRootLite(xml)
  if (!docEl || docEl.tag !== 'document') {
    return { meta: { title: '', summary: '', tags: [] }, blocks: [] }
  }

  const metaEl = firstChild(docEl, 'meta')
  const tagsEl = metaEl ? firstChild(metaEl, 'tags') : undefined
  const meta: DiffMeta = {
    title: metaEl ? fullText(firstChild(metaEl, 'title') ?? { tag: '', attrs: {}, children: [], text: '' }) : '',
    summary: metaEl ? fullText(firstChild(metaEl, 'summary') ?? { tag: '', attrs: {}, children: [], text: '' }) : '',
    tags: tagsEl ? allChildren(tagsEl, 'tag').map(fullText).filter(Boolean) : [],
  }

  const bodyEl = firstChild(docEl, 'body')
  const blocks: DiffBlock[] = []
  const tagCounts = new Map<string, number>()

  const nextOrdinal = (tag: string) => {
    const n = (tagCounts.get(tag) ?? 0) + 1
    tagCounts.set(tag, n)
    return n
  }

  const walk = (parent: XmlLiteNode) => {
    for (const child of parent.children) {
      const id = child.attrs.id?.trim() || undefined
      const prefix = ANCHOR_PREFIX[child.tag]
      const anchorId = id && prefix ? `${prefix}-${id}` : undefined

      if (child.tag === 'section' || child.tag === 'stage') {
        const title = child.attrs.title?.trim() ?? ''
        const ord = nextOrdinal(child.tag)
        if (id || title) {
          blocks.push({
            matchKey: id ? `id:${id}` : `pos:${child.tag}:${ord}`,
            tag: child.tag,
            id,
            anchorId,
            label: blockLabel(child, ord),
            text: title,
          })
        }
        walk(child)
        continue
      }

      if (CONTAINER_TAGS.has(child.tag)) {
        walk(child)
        continue
      }

      if (LEAF_TAGS.has(child.tag)) {
        const ord = nextOrdinal(child.tag)
        blocks.push({
          matchKey: id ? `id:${id}` : `pos:${child.tag}:${ord}`,
          tag: child.tag,
          id,
          anchorId,
          label: blockLabel(child, ord),
          text: serializeLeafBlock(child),
        })
      }
    }
  }

  if (bodyEl) walk(bodyEl)
  return { meta, blocks }
}

function computeTextDiff(oldText: string, newText: string): DiffPart[] {
  const useLines =
    Math.max(oldText.length, newText.length) > 400 ||
    Math.max(oldText.split('\n').length, newText.split('\n').length) >= 6
  const raw = useLines ? diffLines(oldText, newText) : diffWordsWithSpace(oldText, newText)
  return raw.map((part) => ({
    value: part.value,
    ...(part.added ? { added: true } : {}),
    ...(part.removed ? { removed: true } : {}),
  }))
}

/** Compares two parsed Aumbry documents by `<meta>` fields and body blocks (`id` first, then `tag` + ordinal). */
export function diffDocuments(oldDoc: DiffDocument, newDoc: DiffDocument): RevisionDiffResult {
  const metaChanges: MetaFieldChange[] = []
  if (oldDoc.meta.title !== newDoc.meta.title) {
    metaChanges.push({
      field: 'title',
      label: '标题',
      parts: computeTextDiff(oldDoc.meta.title, newDoc.meta.title),
    })
  }
  if (oldDoc.meta.summary !== newDoc.meta.summary) {
    metaChanges.push({
      field: 'summary',
      label: '摘要',
      parts: computeTextDiff(oldDoc.meta.summary, newDoc.meta.summary),
    })
  }
  const oldTagSet = new Set(oldDoc.meta.tags)
  const newTagSet = new Set(newDoc.meta.tags)
  const tagParts: DiffPart[] = []
  for (const t of oldDoc.meta.tags) {
    if (!newTagSet.has(t)) tagParts.push({ value: t, removed: true })
    else tagParts.push({ value: t })
  }
  for (const t of newDoc.meta.tags) {
    if (!oldTagSet.has(t)) tagParts.push({ value: t, added: true })
  }
  if (tagParts.some((p) => p.added || p.removed)) {
    metaChanges.push({ field: 'tags', label: '标签', parts: tagParts })
  }

  const blockChanges: BlockChange[] = []
  const matchedOld = new Set<number>()
  const matchedNew = new Set<number>()

  const oldById = new Map<string, number>()
  oldDoc.blocks.forEach((b, idx) => {
    if (b.id && !oldById.has(b.id)) oldById.set(b.id, idx)
  })

  // Pass 1: pair shared `id` blocks across versions regardless of reordering.
  newDoc.blocks.forEach((nb, nIdx) => {
    if (!nb.id) return
    const oIdx = oldById.get(nb.id)
    if (oIdx === undefined || matchedOld.has(oIdx)) return
    matchedOld.add(oIdx)
    matchedNew.add(nIdx)
    const ob = oldDoc.blocks[oIdx]
    if (ob.text !== nb.text || ob.tag !== nb.tag) {
      blockChanges.push({
        kind: 'modified',
        tag: nb.tag,
        id: nb.id,
        anchorId: nb.anchorId,
        label: nb.label,
        parts: computeTextDiff(ob.text, nb.text),
      })
    }
  })

  // Pass 2: pair remaining unmatched blocks that have NO explicit `id` by `tag` + ordinal.
  // Blocks carrying an explicit `id` whose ID did not match are distinct blocks (removed + added).
  const remainingOldByTag = new Map<string, number[]>()
  oldDoc.blocks.forEach((b, idx) => {
    if (matchedOld.has(idx) || b.id) return
    const list = remainingOldByTag.get(b.tag) ?? []
    list.push(idx)
    remainingOldByTag.set(b.tag, list)
  })
  const consumedPerTag = new Map<string, number>()
  newDoc.blocks.forEach((nb, nIdx) => {
    if (matchedNew.has(nIdx) || nb.id) return
    const list = remainingOldByTag.get(nb.tag) ?? []
    const pos = consumedPerTag.get(nb.tag) ?? 0
    if (pos < list.length) {
      const oIdx = list[pos]
      consumedPerTag.set(nb.tag, pos + 1)
      matchedOld.add(oIdx)
      matchedNew.add(nIdx)
      const ob = oldDoc.blocks[oIdx]
      if (ob.text !== nb.text) {
        blockChanges.push({
          kind: 'modified',
          tag: nb.tag,
          id: nb.id,
          anchorId: nb.anchorId,
          label: nb.label,
          parts: computeTextDiff(ob.text, nb.text),
        })
      }
    }
  })

  // Emit removed and added blocks.
  oldDoc.blocks.forEach((ob, idx) => {
    if (matchedOld.has(idx)) return
    blockChanges.push({
      kind: 'removed',
      tag: ob.tag,
      id: ob.id,
      label: ob.label,
      parts: [{ value: ob.text || ob.label, removed: true }],
    })
  })
  newDoc.blocks.forEach((nb, idx) => {
    if (matchedNew.has(idx)) return
    blockChanges.push({
      kind: 'added',
      tag: nb.tag,
      id: nb.id,
      anchorId: nb.anchorId,
      label: nb.label,
      parts: [{ value: nb.text || nb.label, added: true }],
    })
  })

  const stats = {
    added: blockChanges.filter((c) => c.kind === 'added').length,
    removed: blockChanges.filter((c) => c.kind === 'removed').length,
    modified: blockChanges.filter((c) => c.kind === 'modified').length,
    metaChanged: metaChanges.length,
  }
  return {
    hasChanges: metaChanges.length > 0 || blockChanges.length > 0,
    metaChanges,
    blockChanges,
    stats,
  }
}

/** Parses and diffs two Aumbry XML documents end-to-end. */
export function diffXmlDocuments(oldXml: string, newXml: string): RevisionDiffResult {
  return diffDocuments(parseXmlToDiffDocument(oldXml), parseXmlToDiffDocument(newXml))
}
