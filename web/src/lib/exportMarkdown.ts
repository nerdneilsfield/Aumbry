import TurndownService from 'turndown'
import { gfm } from '@joplin/turndown-plugin-gfm'
import { fence } from '@/lib/fence'

/**
 * "Export Markdown" (PRD 9.3): the rendered, fully expanded article -> GFM Markdown, with turndown. The page is expanded by the
 * caller (usePrint.withExpandedDocument), so locked stages, collapses, tabs, skim folds and steps are all in the DOM. The
 * conversion is DOM-driven: what the print stylesheet hides (`print:hidden`), buttons, icons and elements marked
 * `data-export-skip` are dropped, and the few blocks whose DOM cannot say what they mean carry it themselves:
 *  - `data-export-md`    the Markdown to write instead of the block (Mermaid / chart source, quiz with answers, metrics table,
 *                        checklist, news detail link); it wins over the skip list, so a button can carry one
 *  - `data-export-badge` a label chip, written as `[label] ` before the text it labels
 *  - `data-export-title` a collapse or tab panel: written as a bold title line above its content
 *  - `pre[data-lang]`    the code language of a highlighted block
 *  - `data-tex`          the TeX source of a typeset formula
 */
export interface ExportMeta {
  title: string
  summary?: string
  date?: string
  sourceAgent?: string
  tags?: string[]
  canonicalUrl?: string
}

const SKIP = '[class~="print:hidden"], [data-export-skip], [data-score], button, svg, canvas, .sr-only, [role="tablist"], style, script'
const isElement = (n: Node): n is Element => n.nodeType === 1

function createService(): TurndownService {
  const td = new TurndownService({
    headingStyle: 'atx',
    codeBlockStyle: 'fenced',
    bulletListMarker: '-',
    emDelimiter: '*',
    hr: '---',
    // turndown checks "blank" (no text, no image) before any rule: a chart without caption holds only a canvas or a
    // placeholder, so it would vanish. Blocks that carry their own Markdown are written even when blank.
    blankReplacement: (_content, n) => {
      const md = isElement(n) ? n.getAttribute('data-export-md') : null
      if (md) return `\n\n${md}\n\n`
      return (n as Node & { isBlock?: boolean }).isBlock ? '\n\n' : ''
    },
  })
  td.use(gfm)
  // controls and decoration. A rule, not td.remove(): turndown tries its built-in rules (p, a, h2 ...) before its remove list.
  // Added first, so it has the lowest priority among our rules (turndown: the rule added last wins): a button that declares its
  // own `data-export-md` (the news "read detail" link) is written, not dropped.
  td.addRule('skip', { filter: (n) => isElement(n) && n.matches(SKIP), replacement: () => '' })
  // blocks that write their own Markdown
  td.addRule('exportMd', {
    filter: (n) => isElement(n) && n.hasAttribute('data-export-md'),
    replacement: (_content, n) => `\n\n${(n as Element).getAttribute('data-export-md')}\n\n`,
  })
  // a label chip (evidence level, source) written in front of what it labels
  td.addRule('badge', {
    filter: (n) => isElement(n) && n.hasAttribute('data-export-badge'),
    replacement: (content) => `[${content.trim()}] `,
  })
  td.addRule('titledPanel', {
    filter: (n) => isElement(n) && n.hasAttribute('data-export-title'),
    replacement: (content, n) => `\n\n**${(n as Element).getAttribute('data-export-title')}**\n\n${content.trim()}\n\n`,
  })
  // formulas: the typeset KaTeX DOM is useless as Markdown, the TeX source is carried in data-tex
  td.addRule('math', {
    filter: (n) => isElement(n) && n.hasAttribute('data-tex'),
    replacement: (_content, n) => {
      const tex = (n as Element).getAttribute('data-tex')?.trim() ?? ''
      return (n as Element).querySelector('.katex-display') ? `\n\n$$\n${tex}\n$$\n\n` : `$${tex}$`
    },
  })
  // highlighted code: shiki wraps every line in spans, the text is what counts
  td.addRule('highlightedCode', {
    filter: (n) => n.nodeName === 'PRE' && (n as Element).classList.contains('shiki'),
    replacement: (_content, n) => `\n\n${fence((n as Element).getAttribute('data-lang') ?? '', n.textContent ?? '')}\n\n`,
  })
  return td
}

/** Images and links the article points at on this server (`/api/v1/assets/...`) keep working once the file is saved elsewhere. */
function absolutize(root: Element) {
  for (const [selector, name] of [['img[src]', 'src'], ['a[href]', 'href']] as const) {
    for (const el of root.querySelectorAll(selector)) {
      const raw = el.getAttribute(name) ?? ''
      if (raw.startsWith('#') || !URL.canParse(raw, location.href)) continue
      el.setAttribute(name, new URL(raw, location.href).href)
    }
  }
}

/** GFM Markdown of the article element (clone: the live page is not touched) under the document's title and metadata. */
export function toMarkdown(article: HTMLElement, meta: ExportMeta): string {
  const copy = article.cloneNode(true) as HTMLElement
  absolutize(copy)
  const body = createService().turndown(copy).replace(/\n{3,}/g, '\n\n').trim()
  const facts = [
    meta.date && `日期：${meta.date}`,
    meta.sourceAgent && `来源：${meta.sourceAgent}`,
    meta.tags?.length && `标签：${meta.tags.join('、')}`,
    meta.canonicalUrl && `原文：${meta.canonicalUrl}`,
  ].filter(Boolean)
  const head = [`# ${meta.title}`, meta.summary && `> ${meta.summary}`, facts.length > 0 && facts.join(' · ')].filter(Boolean).join('\n\n')
  return `${head}\n\n---\n\n${body}\n`
}

/** Saves text as a file through a temporary object URL. */
export function downloadText(filename: string, text: string, type = 'text/markdown;charset=utf-8') {
  const url = URL.createObjectURL(new Blob([text], { type }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 0)
}

/** `title` as a file name: no path or reserved characters, at most 80 characters. */
export const markdownFilename = (title: string) => `${title.replace(/[\\/:*?"<>|\u0000-\u001f]+/g, ' ').replace(/\s+/g, ' ').trim().slice(0, 80) || 'aumbry'}.md`
