import { MAX_STAGES_PER_GROUP, anchorMap, stagesStructuralIssues } from '@/lib/anchor'
import { parseAudioTimestamp } from '@/lib/audio'
import { quizCorrect, quizOptions, quizQuestion } from '@/lib/quiz'
import { httpUrl } from '@/lib/url'
import { MAX_SECTION_DEPTH, attr, children, safeId, sectionDepth } from '@/lib/xml'

const bodyOf = (el: Element) => el.closest('body') ?? el.ownerDocument.documentElement

/**
 * Structure and safety checks for the protocol tags, mirroring the error codes of PRD 6.x. The renderer shows each failure
 * inline (so one bad block never hides the rest) and the offline demo lists them all in a banner.
 * Deliberately absent: any rule about which sections exist, what they are called, in what order, or how many.
 */
export interface Issue {
  code: string
  message: string
  /** a short label of the offending element for the banner */
  where?: string
}

const text = (el: Element) => el.textContent?.trim() ?? ''
const kids = (el: Element, tag: string) => children(el).filter((c) => c.tagName === tag)
const clip = (s: string, n = 24) => (s.length > n ? `${s.slice(0, n)}…` : s)
const bytes = (s: string) => new TextEncoder().encode(s).length
/** direct text of an element, without its element children (the body of <news-item> next to its <detail>) */
const ownText = (el: Element) =>
  Array.from(el.childNodes)
    .filter((n) => n.nodeType === 3 || n.nodeType === 4)
    .map((n) => n.textContent ?? '')
    .join('')
    .trim()

export const NEWS_ID = /^[A-Za-z0-9_-]{12}$/
const PUBLISHED = /^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2}))?$/

export function tldrIssues(node: Element): Issue[] {
  const out: Issue[] = []
  const bad = (message: string) => out.push({ code: 'INVALID_TLDR', message })
  const parent = node.parentElement
  const ptag = parent?.tagName ?? ''
  if (!['body', 'section', 'stage'].includes(ptag)) bad('tldr 只能直接放在 body、section 或 stage 里')
  else {
    // anywhere inside body is fine (the whole-document tldr is drawn at the top of the content by default)
    if (kids(parent!, 'tldr')[0] !== node) bad(`同一个 ${ptag} 里至多一个 tldr`)
  }
  const placement = node.getAttribute('placement')
  if (placement !== null && placement !== 'top' && placement !== 'bottom') bad(`placement 必须是 top 或 bottom，收到 "${clip(placement)}"`)
  const points = kids(node, 'point')
  if (points.length < 1 || points.length > 7) bad(`point 有 ${points.length} 个，需要 1–7 个`)
  points.forEach((p, i) => {
    const t = text(p)
    if (!t) bad(`第 ${i + 1} 个 point 是空的`)
    else if ([...t].length > 300) bad(`第 ${i + 1} 个 point 有 ${[...t].length} 个字符，上限 300`)
  })
  const verdicts = kids(node, 'verdict')
  if (verdicts.length > 1) bad('verdict 至多 1 个')
  if (verdicts[0]) {
    const vt = text(verdicts[0])
    if (!vt) bad('verdict 不能为空')
    else if ([...vt].length > 300) bad(`verdict 有 ${[...vt].length} 个字符，上限 300`)
  }
  return out
}

export function newsIssues(node: Element): Issue[] {
  const out: Issue[] = []
  const add = (code: string, message: string) => out.push({ code, message })
  if (!['body', 'section', 'stage'].includes(node.parentElement?.tagName ?? '')) add('INVALID_NEWS', 'news-item 只能直接放在 body、section 或 stage 里')
  if (!attr(node, 'title').trim()) add('INVALID_NEWS', '缺少 title')
  const url = node.getAttribute('url')
  if (url !== null && !httpUrl(url)) add('INVALID_URL', `url 必须是 http(s) 地址，收到 "${clip(url, 40)}"`)
  const id = node.getAttribute('detail-id')
  if (id !== null && !NEWS_ID.test(id)) add('INVALID_ID', `detail-id 必须是 12 位条目 ID（字母、数字、_、-），收到 "${clip(id)}"`)
  const pub = node.getAttribute('published')
  if (pub !== null && !PUBLISHED.test(pub)) add('INVALID_NEWS', `published 必须是 YYYY-MM-DD 或 RFC 3339 时间，收到 "${clip(pub)}"`)
  const imp = node.getAttribute('importance')
  if (imp !== null && imp !== 'high' && imp !== 'normal') add('INVALID_NEWS', `importance 必须是 high 或 normal，收到 "${clip(imp)}"`)
  const n = [...ownText(node)].length
  if (n > 600) add('INVALID_NEWS', `摘要有 ${n} 个字符，上限 600`)
  const details = kids(node, 'detail')
  if (details.length > 1) add('INVALID_NEWS', 'detail 至多 1 个')
  if (details[0] && bytes(text(details[0])) > 8192) add('INVALID_NEWS', 'detail 超过 8 KB')
  return out
}

export function quizIssues(node: Element): Issue[] {
  const out: Issue[] = []
  const add = (message: string) => out.push({ code: 'INVALID_QUIZ', message })
  if (!['body', 'section', 'stage'].includes(node.parentElement?.tagName ?? '')) add('quiz 只能直接放在 body、section 或 stage 里')
  if (!safeId(attr(node, 'id'))) add('缺少 id（[a-z0-9-]{1,32}）')
  if (!quizQuestion(node)) add('缺少非空 question')
  if (quizOptions(node).length < 2 || quizCorrect(node).length < 1) add(`至少 2 个 option 且至少 1 个 correct="true"（现在 ${quizOptions(node).length} 个选项）`)
  return out
}

export function searchLogIssues(node: Element): Issue[] {
  const out: Issue[] = []
  const add = (message: string) => out.push({ code: 'INVALID_SEARCH_LOG', message })
  const queries = kids(node, 'query')
  if (!queries.length) add('至少需要 1 个 query')
  queries.forEach((q, i) => {
    for (const k of ['hits', 'kept']) {
      const v = q.getAttribute(k)
      if (v !== null && !/^\d+$/.test(v)) add(`第 ${i + 1} 个 query 的 ${k} 必须是非负整数`)
    }
    if (!text(q)) add(`第 ${i + 1} 个 query 没有检索式`)
  })
  if (kids(node, 'stop').length > 1) add('stop 至多 1 个')
  return out
}

/** One <stages> group (PRD 6.2/6.7): needs an id, 1–8 well-formed stages, and sits in <body> or a <section>, never inside a <stage>. */
export function stagesIssues(node: Element): Issue[] {
  const out: Issue[] = stagesStructuralIssues(node)
  // ids share one namespace with sections, quizzes, figures, equations and refs
  const dups = anchorMap(bodyOf(node)).dups
  for (const el of [node, ...kids(node, 'stage')]) {
    if (dups.has(el)) out.push({ code: 'DUPLICATE_ID', message: `${el.tagName} 的 id "${clip(attr(el, 'id'))}" 已被文档里更早的元素占用` })
  }
  return out
}

export { MAX_STAGES_PER_GROUP }
export const MAX_STAGES_GROUPS = 8
export const MAX_STAGES = 40
export const MAX_AUDIO_BLOCKS = 4
export const MAX_AUDIO_CHAPTERS = 100
export const MAX_TRANSCRIPT_BYTES = 512 * 1024

/** Validates an `<audio>` block per v1.2 rules: container, src/title/id, chapters monotonic start & optional target, and transcript limit. */
export function audioIssues(node: Element): Issue[] {
  const out: Issue[] = []
  const add = (code: string, message: string) => out.push({ code, message })
  const ptag = node.parentElement?.tagName ?? ''
  if (!['body', 'section', 'stage', 'collapse', 'tab', 'step'].includes(ptag)) {
    add('INVALID_AUDIO', 'audio 只能放在 body、section、stage、collapse、tab 或 step 里')
  }
  if (!attr(node, 'src').trim()) add('INVALID_AUDIO', '缺少 src')
  const title = attr(node, 'title').trim()
  if (!title) add('INVALID_AUDIO', '缺少 title')
  else if ([...title].length > 200) add('INVALID_AUDIO', 'title 超过 200 字符上限')
  const rawId = node.getAttribute('id')
  if (rawId !== null && !safeId(rawId)) add('INVALID_ID', `audio 的 id "${clip(rawId)}" 不符合 [a-z0-9-]{1,32}`)

  const chapters = kids(node, 'chapter')
  if (chapters.length > MAX_AUDIO_CHAPTERS) add('INVALID_AUDIO', `chapter 数量有 ${chapters.length} 个，上限 ${MAX_AUDIO_CHAPTERS}`)
  let prevStart = -1
  chapters.forEach((ch, idx) => {
    const startRaw = attr(ch, 'start').trim()
    const secs = parseAudioTimestamp(startRaw)
    if (secs === null) {
      add('INVALID_AUDIO', `第 ${idx + 1} 个 chapter 的 start "${clip(startRaw)}" 必须是 HH:MM:SS（分秒 00–59）`)
    } else {
      if (prevStart >= 0 && secs < prevStart) {
        add('INVALID_AUDIO', `第 ${idx + 1} 个 chapter 的 start "${startRaw}" 小于上一章时间`)
      }
      prevStart = secs
    }
    if (!attr(ch, 'title').trim()) add('INVALID_AUDIO', `第 ${idx + 1} 个 chapter 缺少 title`)
    const target = ch.getAttribute('target')
    if (target !== null && !nextStepTarget(node, target)) {
      add('INVALID_TARGET', `第 ${idx + 1} 个 chapter 的 target "${clip(target, 40)}" 不是本文档里存在的锚点`)
    }
  })

  const transcripts = kids(node, 'transcript')
  if (transcripts.length > 1) add('INVALID_AUDIO', 'transcript 至多 1 个')
  if (transcripts[0]) {
    if (transcripts[0].children.length > 0) add('INVALID_AUDIO', 'transcript 内不允许嵌套 XML 标签')
    const txt = text(transcripts[0])
    if (!txt) add('INVALID_AUDIO', 'transcript 存在时不能为空')
    else if (bytes(txt) > MAX_TRANSCRIPT_BYTES) add('INVALID_AUDIO', 'transcript 超过 512 KB 上限')
  }
  return out
}

/** A next-steps target: `^#[a-z0-9-]+$` resolving through the prefix table (s- stages- stage- quiz- fig- eq- ref- audio-) to an existing element. */
export const nextStepTarget = (from: Element, target: string) => !!anchorMap(bodyOf(from)).findStrict(target)

export function nextStepsIssues(node: Element): Issue[] {
  const out: Issue[] = []
  const opts = kids(node, 'option')
  if (opts.length < 1 || opts.length > 5) out.push({ code: 'INVALID_NEXT_STEPS', message: `option 有 ${opts.length} 个，需要 1–5 个` })
  for (const o of opts) {
    const t = attr(o, 'target')
    if (!nextStepTarget(node, t)) out.push({ code: 'INVALID_TARGET', message: `target "${clip(t, 40)}" 不是本文档里存在的锚点（形如 #stage-r2、#fig-arch、#s-intro、#quiz-q1，不含 #sec-N）` })
  }
  return out
}

/** every issue in a document, for the demo banner */
export function validateDoc(body: Element): Issue[] {
  const out: Issue[] = []
  const add = (where: string, issues: Issue[]) => issues.forEach((i) => out.push({ ...i, where }))
  const all = (sel: string) => Array.from(body.querySelectorAll(sel))

  for (const t of all('tldr')) add(`tldr「${clip(text(kids(t, 'point')[0] ?? t))}」`, tldrIssues(t))
  for (const n of all('news-item')) add(`news-item「${clip(attr(n, 'title'))}」`, newsIssues(n))
  for (const q of all('quiz')) add(`quiz「${clip(attr(q, 'id') || text(kids(q, 'question')[0] ?? q))}」`, quizIssues(q))
  for (const s of all('search-log')) add('search-log', searchLogIssues(s))
  for (const n of all('next-steps')) add('next-steps', nextStepsIssues(n))
  const audios = all('audio')
  if (audios.length > 0 && body.ownerDocument?.documentElement?.getAttribute('version') === '1.1') {
    out.push({ code: 'INVALID_AUDIO', message: '<audio> 仅在 <document version="1.2"> 中受支持，请升级文档版本', where: 'audio' })
  }
  if (audios.length > MAX_AUDIO_BLOCKS) {
    out.push({ code: 'INVALID_AUDIO', message: `一份文档最多 ${MAX_AUDIO_BLOCKS} 个 audio 块`, where: 'audio' })
  }
  for (const a of audios) add(`audio「${clip(attr(a, 'title'))}」`, audioIssues(a))

  for (const s of all('section')) {
    if (sectionDepth(s) > MAX_SECTION_DEPTH) out.push({ code: 'SECTION_TOO_DEEP', message: `section 最多嵌套 ${MAX_SECTION_DEPTH} 层`, where: `section「${clip(attr(s, 'title'))}」` })
  }

  for (const s of all('stages')) add(`stages「${clip(attr(s, 'id'))}」`, stagesIssues(s))
  if (all('stages').length > MAX_STAGES_GROUPS) out.push({ code: 'TOO_MANY_BLOCKS', message: `一份文档最多 ${MAX_STAGES_GROUPS} 组 stages`, where: 'stages' })
  if (all('stage').length > MAX_STAGES) out.push({ code: 'TOO_MANY_BLOCKS', message: `一份文档最多 ${MAX_STAGES} 个 stage`, where: 'stage' })

  for (const c of all('code-block[eq]')) {
    if (!anchorMap(body).byDom.has(`eq-${c.getAttribute('eq')}`)) out.push({ code: 'INVALID_TARGET', message: `eq="${clip(attr(c, 'eq'))}" 没有对应的 equation`, where: 'code-block' })
  }

  // one id namespace for section / stages / stage / quiz / figure / equation / ref, across types
  for (const [e, id] of anchorMap(body).dups) if (e.tagName !== 'stages' && e.tagName !== 'stage') out.push({ code: 'DUPLICATE_ID', message: `id "${clip(id)}" 已被文档里更早的元素占用`, where: e.tagName })
  return out
}
