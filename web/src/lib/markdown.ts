import MarkdownIt from 'markdown-it'
import texmath from 'markdown-it-texmath'
import katex from 'katex'
import DOMPurify from 'dompurify'
import { dedent } from 'ts-dedent'
import { createHighlighterCore } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
import githubDark from 'shiki/themes/github-dark.mjs'
import 'katex/dist/katex.min.css'

// Top-level await: only the lazily loaded reader chunk imports this module.
const highlighter = await createHighlighterCore({
  themes: [githubDark],
  langs: [
    import('shiki/langs/bash.mjs'),
    import('shiki/langs/c.mjs'),
    import('shiki/langs/cpp.mjs'),
    import('shiki/langs/css.mjs'),
    import('shiki/langs/diff.mjs'),
    import('shiki/langs/go.mjs'),
    import('shiki/langs/html.mjs'),
    import('shiki/langs/javascript.mjs'),
    import('shiki/langs/json.mjs'),
    import('shiki/langs/markdown.mjs'),
    import('shiki/langs/python.mjs'),
    import('shiki/langs/rust.mjs'),
    import('shiki/langs/sql.mjs'),
    import('shiki/langs/toml.mjs'),
    import('shiki/langs/typescript.mjs'),
    import('shiki/langs/yaml.mjs'),
  ],
  engine: createJavaScriptRegexEngine(),
})
const loaded = new Set(highlighter.getLoadedLanguages())

// html:false is the XSS boundary: raw HTML in markdown is escaped, never emitted.
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: false,
  highlight: (code, lang) =>
    loaded.has(lang) ? highlighter.codeToHtml(code, { lang, theme: 'github-dark' }) : '',
})
md.use(texmath, { engine: katex, delimiters: 'dollars', katexOptions: { throwOnError: false } })

// Defence in depth on top of markdown-it's own filter: http(s), same-origin paths and anchors only.
md.validateLink = (url) => /^(https?:\/\/|\/(?!\/)|#)/i.test(url.trim())

// Second line of defence: sanitise the final HTML. texmath wraps formulas in <eq>/<eqn>.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && /^https?:/i.test(node.getAttribute('href') ?? '')) {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer')
  }
})

export const renderMarkdown = (src: string) =>
  DOMPurify.sanitize(md.render(dedent(src)), { ADD_TAGS: ['eq', 'eqn', 'eqno'], ADD_ATTR: ['target'] })
