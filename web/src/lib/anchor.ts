/**
 * Ids and anchors of an Aumbry document (PRD 6.4). No imports: everything else (xml, quiz, validate, the renderer) builds on this.
 *
 * One shared id namespace covers <section>, <stages>, <stage>, <quiz>, <figure>, <equation> and <ref>; the first element
 * to use an id owns it, any later one (even of another type) is a duplicate (DUPLICATE_ID, shown as an error block).
 * DOM ids / anchors:  section `#s-<id>`, stages `#stages-<id>`, stage `#stage-<id>`, quiz `#quiz-<id>`,
 * figure `#fig-<id>`, equation `#eq-<id>`, ref `#ref-<id>`. The positional `#sec-N` is kept for the TOC and scrolling
 * only (it is not a valid next-steps target), as is `#stage=N` (the Nth stage of the first group).
 */

/** `id` values are limited to [a-z0-9-]{1,32} by the protocol; anything else never becomes a DOM id. */
export const safeId = (raw: string | null | undefined) => (raw && /^[a-z0-9-]{1,32}$/.test(raw) ? raw : '')

export type AnchorKind = 'section' | 'stages' | 'stage' | 'quiz' | 'figure' | 'equation' | 'ref'
const PREFIX: Record<AnchorKind, string> = { section: 's', stages: 'stages', stage: 'stage', quiz: 'quiz', figure: 'fig', equation: 'eq', ref: 'ref' }
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
  if (tag === 'quiz') return `quiz-${quizKey(el)}`
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
  const owner = new Map<string, Element>()

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

  for (const el of body.querySelectorAll('*')) {
    const tag = el.tagName
    if (!isKind(tag)) continue
    const id = safeId(el.getAttribute('id'))
    if (id) {
      if (owner.has(id)) {
        dups.set(el, id)
        continue
      }
      owner.set(id, el)
    }
    const domId = domIdOf(el)
    if (domId) byDom.set(domId, info(el, tag, domId))
    // sections are always reachable by position too (TOC / scroll)
    if (tag === 'section' && id) byDom.set(`sec-${position(el, 'section')}`, info(el, tag, domId))
  }

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
    find: (hash) => {
      const id = decode(hash)
      const st = /^stage=(\d+)$/.exec(id)
      if (st) {
        const first = body.querySelector('stages')
        const el = first && Array.from(first.children).filter((c) => c.tagName === 'stage')[Number(st[1]) - 1]
        return el ? byDom.get(stageDomId(el)) : undefined
      }
      return byDom.get(id)
    },
    findStrict: (hash) => (/^#[a-z0-9-]+$/.test(hash) && !/^#sec-\d+$/.test(hash) ? byDom.get(hash.slice(1)) : undefined),
  }
  cache.set(body, map)
  return map
}
