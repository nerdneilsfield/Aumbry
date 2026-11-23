/**
 * Ids and anchors of an Aumbry document (PRD 6.4). No imports: everything else (xml, quiz, validate, the renderer) builds on this.
 *
 * One shared id namespace covers <section>, <stages>, <stage>, <quiz>, <figure>, <equation>, <ref>, <audio> and <attachment>; the first element
 * to use an id owns it, any later one (even of another type) is a duplicate (DUPLICATE_ID, shown as an error block).
 * DOM ids / anchors:  section `#s-<id>`, stages `#stages-<id>`, stage `#stage-<id>`, quiz `#quiz-<id>`,
 * figure `#fig-<id>`, equation `#eq-<id>`, ref `#ref-<id>`, audio `#audio-<id>`, attachment `#att-<id>` (and `#att-<id>:p=<N>`). The positional `#sec-N` is kept for the TOC and scrolling
 * only (it is not a valid next-steps target), as is `#stage=N` (the Nth stage of the first group).
 */

export const MAX_SECTION_DEPTH = 3
export const MAX_STAGES_PER_GROUP = 8

/** how many <section> ancestors (plus itself) a section has: 1 = top level; the protocol allows 3 */
export const sectionDepth = (el: Element) => {
  let d = 1
  for (let p = el.parentElement; p; p = p.parentElement) if (p.tagName === 'section') d++
  return d
}

const directKids = (el: Element, tag: string) => Array.from(el.children).filter((c) => c.tagName === tag)

/** Non-duplicate structural issues of a <stages> group; shared by anchorMap, validate.ts and buildToc. */
export function stagesStructuralIssues(node: Element): { code: string; message: string }[] {
  const out: { code: string; message: string }[] = []
  const add = (message: string) => out.push({ code: 'INVALID_STAGES', message })
  if (!safeId(node.getAttribute('id'))) add('缺少 id（[a-z0-9-]{1,32}），阅读进度按它保存')
  const parent = node.parentElement
  if (parent?.closest('stage')) add('stages 不能放在 stage 里面（不能嵌套）')
  else if (!['body', 'section'].includes(parent?.tagName ?? '')) add('stages 只能放在 body 或 section 里')
  const stages = directKids(node, 'stage')
  const n = stages.length
  if (n < 1 || n > MAX_STAGES_PER_GROUP) add(`一组 stages 需要 1–${MAX_STAGES_PER_GROUP} 个 stage，现在 ${n} 个`)
  stages.forEach((st, i) => {
    if (!safeId(st.getAttribute('id'))) add(`第 ${i + 1} 个 stage 缺少有效 id（[a-z0-9-]{1,32}）`)
    if (!(st.getAttribute('title') ?? '').trim()) add(`第 ${i + 1} 个 stage 缺少 title`)
  })
  const gated = node.getAttribute('gated')
  if (gated !== null && gated !== 'true' && gated !== 'false') add(`gated 必须是 true 或 false，收到 "${gated.slice(0, 24)}"`)
  return out
}

/** `id` values are limited to [a-z0-9-]{1,32} by the protocol; anything else never becomes a DOM id. */
export const safeId = (raw: string | null | undefined) => (raw && /^[a-z0-9-]{1,32}$/.test(raw) ? raw : '')

export type AnchorKind = 'section' | 'stages' | 'stage' | 'quiz' | 'figure' | 'equation' | 'ref' | 'audio' | 'attachment'
const PREFIX: Record<AnchorKind, string> = { section: 's', stages: 'stages', stage: 'stage', quiz: 'quiz', figure: 'fig', equation: 'eq', ref: 'ref', audio: 'audio', attachment: 'att' }

/** Splits an optional `:p=<N>` PDF page suffix from an anchor hash (e.g. `#att-paper:p=4`). */
export function parseAnchorTarget(raw: string): { base: string; page?: number } | null {
  const idx = raw.indexOf(':p=')
  if (idx !== -1) {
    const base = raw.slice(0, idx)
    const pageStr = raw.slice(idx + 3)
    if (!/^#att-[a-z0-9-]+$/.test(base) || !/^[1-9][0-9]*$/.test(pageStr)) return null
    const page = Number.parseInt(pageStr, 10)
    return Number.isSafeInteger(page) && page >= 1 ? { base, page } : null
  }
  return /^#[a-z0-9-]+$/.test(raw) ? { base: raw } : null
}
const KINDS = Object.keys(PREFIX) as AnchorKind[]
const isKind = (tag: string): tag is AnchorKind => (KINDS as string[]).includes(tag)

const position = (el: Element, selector: string) => Array.from(el.ownerDocument.querySelectorAll(selector)).indexOf(el)

/** DOM id of a section: `s-<id>` when it has an id, else the positional `sec-N`. */
export const sectionId = (el: Element) => (safeId(el.getAttribute('id')) ? `s-${el.getAttribute('id')}` : `sec-${position(el, 'section')}`)
/** DOM id of a stage: `stage-<id>` when it has an id, else `stage-<position among all stages>`. */
export const stageDomId = (el: Element) => `stage-${safeId(el.getAttribute('id')) || position(el, 'stages > stage') + 1}`
/** State key / DOM id suffix of a quiz: its id, else `n<position>` (an id-less quiz is invalid and never stores state). */
export const quizKey = (el: Element) => safeId(el.getAttribute('id')) || `n${position(el, 'quiz') + 1}`

export function domIdOf(el: Element): string {
  const tag = el.tagName
  if (tag === 'section') return sectionId(el)
  if (tag === 'stage') return stageDomId(el)
  if (tag === 'quiz') return safeId(el.getAttribute('id')) ? `quiz-${quizKey(el)}` : ''
  if (!isKind(tag)) return ''
  const id = safeId(el.getAttribute('id'))
  return id ? `${PREFIX[tag]}-${id}` : ''
}

/** A target and everything that has to be opened before it is on screen. */
export interface AnchorInfo {
  el: Element
  kind: AnchorKind
  domId: string
  /** the <stage> elements the target sits in (a stage target includes itself): their groups unlock up to them */
  stages: Element[]
  collapses: Element[]
  /** <tab> elements on the path (the tab group switches to them) */
  tabs: Element[]
  /** <step> elements of interactive <steps> on the path */
  steps: Element[]
  /** skim folds to open: the target itself when it is a section / stage, and the container holding it */
  unfold: Element[]
}

export interface AnchorMap {
  byDom: Map<string, AnchorInfo>
  /** elements whose id was already used by an earlier element of the shared namespace */
  dups: Map<Element, string>
  /** whether el (or any ancestor) lives in an unrendered subtree (too-deep section, duplicate-id container, or invalid stages) */
  isSkipped: (el: Element) => boolean
  /** the target of `#hash` (non-strict: also `#sec-N`) */
  find: (hash: string) => AnchorInfo | undefined
  /** next-steps targets: `^#[a-z0-9-]+$` resolving through the prefix table to an existing element (not `#sec-N`) */
  findStrict: (hash: string) => AnchorInfo | undefined
}

const cache = new WeakMap<Element, AnchorMap>()

/** Built once per parsed document (cached on the <body> element). */
export function anchorMap(body: Element): AnchorMap {
  const hit = cache.get(body)
  if (hit) return hit
  const byDom = new Map<string, AnchorInfo>()
  const dups = new Map<Element, string>()
  const skipped = new Set<Element>()
  const owner = new Map<string, Element>()
  const renderedStages: Element[] = []

  const info = (el: Element, kind: AnchorKind, domId: string): AnchorInfo => {
    const stages: Element[] = kind === 'stage' ? [el] : []
    const collapses: Element[] = []
    const tabs: Element[] = []
    const steps: Element[] = []
    const unfold: Element[] = kind === 'section' || kind === 'stage' ? [el] : []
    // a section / stage target unfolds itself only; any other target unfolds the container that holds it
    let container = kind === 'section' || kind === 'stage'
    for (let p = el.parentElement; p; p = p.parentElement) {
      const t = p.tagName
      if (t === 'stage') stages.push(p)
      else if (t === 'collapse') collapses.push(p)
      else if (t === 'tab') tabs.push(p)
      else if (t === 'step' && p.parentElement?.tagName === 'steps' && p.parentElement.getAttribute('interactive') === 'true') steps.push(p)
      if (!container && (t === 'section' || t === 'stage' || t === 'body')) {
        unfold.push(p)
        container = true
      }
    }
    return { el, kind, domId, stages, collapses, tabs, steps, unfold }
  }

  // Recursive walk mirroring what the renderer actually mounts: unrendered subtrees (section depth > 3,
  // duplicate-id containers, structurally invalid or id-colliding <stages>) are skipped before claiming any ids.
  const walk = (parent: Element, secDepth: number) => {
    for (const el of Array.from(parent.children)) {
      const tag = el.tagName
      if (tag === 'section') {
        const nextDepth = secDepth + 1
        if (nextDepth > MAX_SECTION_DEPTH) {
          skipped.add(el)
          continue
        }
        const id = safeId(el.getAttribute('id'))
        if (id) {
          if (owner.has(id)) {
            dups.set(el, id)
            skipped.add(el)
            continue
          }
          owner.set(id, el)
        }
        const domId = domIdOf(el)
        if (domId) byDom.set(domId, info(el, 'section', domId))
        if (id) byDom.set(`sec-${position(el, 'section')}`, info(el, 'section', domId))
        walk(el, nextDepth)
        continue
      }
      if (tag === 'stages') {
        const stageKids = directKids(el, 'stage')
        const groupIds = new Set<string>()
        let hasDup = false
        for (const node of [el, ...stageKids]) {
          const id = safeId(node.getAttribute('id'))
          if (!id) continue
          if (owner.has(id) || groupIds.has(id)) {
            dups.set(node, id)
            hasDup = true
          } else {
            groupIds.add(id)
          }
        }
        if (stagesStructuralIssues(el).length > 0 || hasDup) {
          skipped.add(el)
          continue
        }
        const gid = safeId(el.getAttribute('id'))
        owner.set(gid, el)
        const gDom = domIdOf(el)
        if (gDom) byDom.set(gDom, info(el, 'stages', gDom))
        renderedStages.push(el)
        for (const st of stageKids) {
          const sid = safeId(st.getAttribute('id'))
          owner.set(sid, st)
          const sDom = domIdOf(st)
          if (sDom) byDom.set(sDom, info(st, 'stage', sDom))
          walk(st, secDepth)
        }
        continue
      }
      if (tag === 'stage') {
        // Standalone <stage> outside a <stages> container is never rendered.
        skipped.add(el)
        continue
      }
      if (isKind(tag)) {
        const id = safeId(el.getAttribute('id'))
        if (id) {
          if (owner.has(id)) {
            dups.set(el, id)
            skipped.add(el)
            continue
          }
          owner.set(id, el)
        }
        const domId = domIdOf(el)
        if (domId) byDom.set(domId, info(el, tag, domId))
      }
      walk(el, secDepth)
    }
  }
  walk(body, 0)

  const decode = (hash: string) => {
    const raw = hash.replace(/^#/, '')
    try {
      return decodeURIComponent(raw)
    } catch {
      return raw
    }
  }
  const map: AnchorMap = {
    byDom,
    dups,
    isSkipped: (el) => {
      for (let p: Element | null = el; p && p !== body; p = p.parentElement) {
        if (skipped.has(p)) return true
      }
      return false
    },
    find: (hash) => {
      const id = decode(hash)
      const st = /^stage=(\d+)$/.exec(id)
      if (st) {
        const first = renderedStages[0]
        const el = first && directKids(first, 'stage')[Number(st[1]) - 1]
        return el ? byDom.get(stageDomId(el)) : undefined
      }
      const parsed = parseAnchorTarget(id.startsWith('#') ? id : `#${id}`)
      if (parsed) return byDom.get(parsed.base.slice(1))
      return byDom.get(id)
    },
    findStrict: (hash) => {
      const parsed = parseAnchorTarget(hash)
      if (!parsed || /^#sec-\d+$/.test(parsed.base)) return undefined
      return byDom.get(parsed.base.slice(1))
    },
  }
  cache.set(body, map)
  return map
}
