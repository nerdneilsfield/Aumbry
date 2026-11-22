import MarkdownIt from 'markdown-it'
import footnote from 'markdown-it-footnote'
import taskLists from 'markdown-it-task-lists'
import sub from 'markdown-it-sub'
import sup from 'markdown-it-sup'
import texmath from 'markdown-it-texmath'
import katex from 'katex'
import DOMPurify from 'dompurify'
import { dedent } from 'ts-dedent'
import { dualTheme, highlighter, loadedLangs } from '@/lib/highlighter'
import { safeUrl } from '@/lib/url'
import 'katex/dist/katex.min.css'

// html:false is the XSS boundary: raw HTML in markdown is escaped, never emitted.
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: false,
  highlight: (code, lang) =>
    loadedLangs.has(lang) ? highlighter.codeToHtml(code, { lang, ...dualTheme }) : '',
})
md.use(texmath, { engine: katex, delimiters: 'dollars', katexOptions: { throwOnError: false } })
md.use(footnote)
md.use(taskLists, { enabled: false })
md.use(sub)
md.use(sup)

// Defence in depth on top of markdown-it's own filter: http(s), same-origin URLs and anchors only (see safeUrl).
md.validateLink = (url) => !!safeUrl(url)

// Second line of defence: sanitise the final HTML with the same rule. texmath wraps formulas in <eq>/<eqn>;
// task-lists emits read-only <input type="checkbox" disabled>. Only checkbox inputs with disabled are retained.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'INPUT') {
    if (node.getAttribute('type') !== 'checkbox' || !node.hasAttribute('disabled')) {
      node.remove()
      return
    }
    for (const a of Array.from(node.attributes)) {
      if (!['type', 'disabled', 'checked', 'class'].includes(a.name)) node.removeAttribute(a.name)
    }
    return
  }
  node.removeAttribute('disabled')
  node.removeAttribute('checked')
  for (const name of ['href', 'src']) {
    const v = node.getAttribute(name)
    if (v !== null && !safeUrl(v)) node.removeAttribute(name)
  }
  if (node.tagName === 'A' && /^https?:/i.test(node.getAttribute('href') ?? '')) {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer')
  }
})

const clean = (html: string) =>
  DOMPurify.sanitize(html, { ADD_TAGS: ['eq', 'eqn', 'eqno', 'sub', 'sup', 'input'], ADD_ATTR: ['target', 'type', 'disabled', 'checked'] })

let mdRenderSeq = 0

export const renderMarkdown = (src: string, docId?: string) => clean(md.render(dedent(src), { docId: docId ?? `md${++mdRenderSeq}` }))

/** One line of Markdown (<tldr> points, <news-item> summaries): inline rules only, no block elements. */
export const renderInline = (src: string) => clean(md.renderInline(src.trim()))

/** Inline Markdown reduced to plain text (TOC summaries, tooltips). */
export const inlineText = (src: string) =>
  (md.parseInline(src.trim(), {})[0]?.children ?? [])
    .map((t) => (t.type === 'softbreak' || t.type === 'hardbreak' ? ' ' : ['text', 'code_inline', 'math_inline'].includes(t.type) ? t.content : ''))
    .join('')
