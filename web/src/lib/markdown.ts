import DOMPurify from 'dompurify'
import { dualTheme, highlighter, langAttribute, loadedLangs } from '@/lib/highlighter'
import { createMarkdownEngine, extractInlineText, renderInlineRaw, renderMarkdownRaw } from '@/lib/markdownEngine'
import { safeUrl } from '@/lib/url'
import 'katex/dist/katex.min.css'

// Defence in depth on top of markdown-it's own filter: http(s), same-origin URLs and anchors only (see safeUrl).
const md = createMarkdownEngine({
  highlight: (code, lang) =>
    loadedLangs.has(lang) ? highlighter.codeToHtml(code, { lang, ...dualTheme, transformers: [langAttribute] }) : '',
  validateLink: (url) => !!safeUrl(url),
})

// Second line of defence: sanitise the final HTML with the same rule. texmath and bracketMath wrap formulas in <eq>/<eqn>;
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

export const renderMarkdown = (src: string, docId?: string) => clean(renderMarkdownRaw(md, src, docId ?? `md${++mdRenderSeq}`))

/** One line of Markdown (<tldr> points, <news-item> summaries): inline rules only, no block elements. */
export const renderInline = (src: string) => clean(renderInlineRaw(md, src))

/** Inline Markdown reduced to plain text (TOC summaries, tooltips). */
export const inlineText = (src: string) => extractInlineText(md, src)
