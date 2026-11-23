import MarkdownIt from 'markdown-it'
import type { MarkdownIt as MarkdownItInstance, MarkdownItOptions } from 'markdown-it'
import footnote from 'markdown-it-footnote'
import taskLists from 'markdown-it-task-lists'
import sub from 'markdown-it-sub'
import sup from 'markdown-it-sup'
import texmath from 'markdown-it-texmath'
import katex from 'katex'
import { dedent } from 'ts-dedent'

const KATEX_OPTIONS = {
  throwOnError: false,
  trust: false,
  strict: 'ignore',
  maxExpand: 1000,
  maxSize: 30,
} as const

// Only an optional standalone equation number `(eqno)` may follow closing `\]` on the same line,
// so escaped intervals with trailing prose like `\[-1.4; +4.3\] 是置信区间` are left untouched.
const TRAILING_EQNO_RE = /^[ \t]*(?:\(([^)\s]+)\)[ \t]*)?$/

/** Finds `marker` in `str[from..limit)` only when preceded by an even number of backslashes. */
function findUnescaped(str: string, marker: string, from = 0, limit = str.length): number {
  let idx = str.indexOf(marker, from)
  while (idx !== -1 && idx + marker.length <= limit) {
    let backslashes = 0
    for (let i = idx - 1; i >= from && str.charCodeAt(i) === 0x5c; i--) backslashes++
    if (backslashes % 2 === 0) return idx
    idx = str.indexOf(marker, idx + marker.length)
  }
  return -1
}

/**
 * Adds LaTeX bracket math rules (`\(...\)` inline, `\[...\]` block).
 * Unlike texmath's built-in `brackets` preset, `math_bracket_block` interrupts paragraphs/headings/lists,
 * handles LaTeX line-break spacing like `\\[4pt]` inside aligned blocks, and rejects trailing prose
 * after `\]` so escaped brackets remain plain text.
 */
function bracketMath(md: MarkdownItInstance, renderMath: (tex: string, displayMode: boolean) => string) {
  md.inline.ruler.before('escape', 'math_bracket_inline', (state, silent) => {
    const pos = state.pos
    const max = state.posMax
    const src = state.src
    if (pos + 1 >= max || src.charCodeAt(pos) !== 0x5c || src.charCodeAt(pos + 1) !== 0x28) return false

    const close = findUnescaped(src, '\\)', pos + 2, max)
    if (close === -1) return false

    const nested = findUnescaped(src, '\\(', pos + 2, close)
    if (nested !== -1) return false

    const content = src.slice(pos + 2, close)
    if (!content.trim()) return false

    if (!silent) {
      const token = state.push('math_bracket_inline', 'math', 0)
      token.content = content
      token.markup = '\\('
    }
    state.pos = close + 2
    return true
  })

  md.block.ruler.before(
    'fence',
    'math_bracket_block',
    (state, startLine, endLine, silent) => {
      if (state.sCount[startLine] - state.blkIndent >= 4) return false
      const startPos = state.bMarks[startLine] + state.tShift[startLine]
      const startMax = state.eMarks[startLine]
      if (state.src.slice(startPos, startPos + 2) !== '\\[') return false

      const firstRest = state.src.slice(startPos + 2, startMax)
      let closeLine = startLine
      let content = ''
      let eqno = ''

      const sameLineClose = findUnescaped(firstRest, '\\]')
      if (sameLineClose !== -1) {
        if (findUnescaped(firstRest, '\\[', 0, sameLineClose) !== -1) return false
        const match = TRAILING_EQNO_RE.exec(firstRest.slice(sameLineClose + 2))
        if (!match) return false
        content = firstRest.slice(0, sameLineClose)
        eqno = match[1] ?? ''
      } else {
        if (findUnescaped(firstRest, '\\[') !== -1) return false
        const lines: string[] = firstRest.trim() ? [firstRest] : []
        let found = false
        for (let line = startLine + 1; line < endLine; line++) {
          if (state.isEmpty(line)) {
            lines.push('')
            continue
          }
          if (state.sCount[line] < state.blkIndent) break
          const lStart = state.bMarks[line] + state.tShift[line]
          const lMax = state.eMarks[line]
          const text = state.src.slice(lStart, lMax)
          if (state.sCount[line] - state.blkIndent < 4 && /^(?:`{3,}|~{3,})/.test(text)) break

          const closeIdx = findUnescaped(text, '\\]')
          const nestedIdx = findUnescaped(text, '\\[')
          if (closeIdx !== -1) {
            if (nestedIdx !== -1 && nestedIdx < closeIdx) break
            const match = TRAILING_EQNO_RE.exec(text.slice(closeIdx + 2))
            if (!match) break
            const before = text.slice(0, closeIdx)
            if (before.trim()) lines.push(before)
            closeLine = line
            eqno = match[1] ?? ''
            found = true
            break
          }
          if (nestedIdx !== -1) break
          lines.push(text)
        }
        if (!found) return false
        content = lines.join('\n')
      }

      if (!content.trim()) return false
      if (silent) return true

      const oldParent = state.parentType
      const oldLineMax = state.lineMax
      state.lineMax = closeLine
      state.parentType = 'math' as typeof state.parentType
      const token = state.push(eqno ? 'math_bracket_block_eqno' : 'math_bracket_block', 'math', 0)
      token.block = true
      token.content = content
      token.info = eqno
      token.markup = '\\['
      token.map = [startLine, closeLine + 1]
      state.parentType = oldParent
      state.lineMax = oldLineMax
      state.line = closeLine + 1
      return true
    },
    { alt: ['paragraph', 'reference', 'blockquote', 'list'] },
  )

  md.renderer.rules.math_bracket_inline = (tokens, idx) => `<eq>${renderMath(tokens[idx].content, false)}</eq>`
  md.renderer.rules.math_bracket_block = (tokens, idx) => `<section><eqn>${renderMath(tokens[idx].content, true)}</eqn></section>`
  md.renderer.rules.math_bracket_block_eqno = (tokens, idx) =>
    `<section class="eqno"><eqn>${renderMath(tokens[idx].content, true)}</eqn><span>(${md.utils.escapeHtml(tokens[idx].info)})</span></section>`
}

export function createMarkdownEngine(opts?: {
  highlight?: MarkdownItOptions['highlight']
  validateLink?: (url: string) => boolean
}): MarkdownItInstance {
  // html:false is the XSS boundary: raw HTML in markdown is escaped, never emitted.
  const md = new MarkdownIt({
    html: false,
    linkify: true,
    breaks: false,
    highlight: opts?.highlight,
  })
  // The typeset formula carries its TeX source in data-tex (Markdown export reads it back; the KaTeX DOM cannot be turned into TeX).
  const renderMath = (tex: string, displayMode: boolean) => {
    try {
      return `<span data-tex="${md.utils.escapeHtml(tex)}">${katex.renderToString(tex, { ...KATEX_OPTIONS, displayMode })}</span>`
    } catch (err) {
      return `<span class="katex-error">${md.utils.escapeHtml(`${tex}: ${err instanceof Error ? err.message : String(err)}`)}</span>`
    }
  }
  const katexWithSource = {
    renderToString: (tex: string, options: object) =>
      `<span data-tex="${md.utils.escapeHtml(tex)}">${katex.renderToString(tex, options)}</span>`,
  }
  // texmath mutates katexOptions.displayMode in place, so pass a fresh copy.
  md.use(texmath, { engine: katexWithSource, delimiters: 'dollars', katexOptions: { ...KATEX_OPTIONS } })
  bracketMath(md, renderMath)
  md.use(footnote)
  md.use(taskLists, { enabled: false })
  md.use(sub)
  md.use(sup)
  if (opts?.validateLink) md.validateLink = opts.validateLink
  return md
}

export const renderMarkdownRaw = (md: MarkdownItInstance, src: string, docId: string) => md.render(dedent(src), { docId })

export const renderInlineRaw = (md: MarkdownItInstance, src: string) => md.renderInline(src.trim())

export const extractInlineText = (md: MarkdownItInstance, src: string) =>
  (md.parseInline(src.trim(), {})[0]?.children ?? [])
    .map((t) =>
      t.type === 'softbreak' || t.type === 'hardbreak'
        ? ' '
        : ['text', 'code_inline', 'math_inline', 'math_inline_double', 'math_bracket_inline'].includes(t.type)
          ? t.content
          : '',
    )
    .join('')
