import type { ShikiTransformer } from 'shiki/core'
import { transformerMetaHighlight, transformerNotationDiff, transformerNotationHighlight } from '@shikijs/transformers'
import { dualTheme, highlighter, loadedLangs } from '@/lib/highlighter'

/**
 * Highlights code with shiki (light+dark dual theme). The language grammar does the colouring:
 *  - a unified diff (`diff="true"` or `lang="diff"`, leading +/- on each line) uses shiki's built-in `diff` language, as is;
 *  - `highlight="3,5-7"` marks lines through @shikijs/transformers' meta-highlight (`{3,5-7}`), `// [!code highlight]` marks
 *    one line, `// [!code ++]` / `// [!code --]` mark diff lines in ordinary code (those directives only read comments).
 * Returns HTML produced by shiki, which escapes the code itself.
 */
export function highlightCode(source: string, lang: string, diff: boolean, highlight = ''): string {
  const l = diff ? 'diff' : loadedLangs.has(lang) ? lang : 'text'
  const rows = source.split('\n')
  const transformers: ShikiTransformer[] = [transformerNotationDiff(), transformerNotationHighlight(), transformerMetaHighlight()]
  if (l === 'diff') {
    // the diff grammar colours the text; this also tints the whole line so added / removed lines read at a glance
    transformers.push({
      name: 'aumbry-diff-lines',
      pre(node) {
        this.addClassToHast(node, 'has-diff')
      },
      line(node, line) {
        const row = rows[line - 1] ?? ''
        if (/^(\+\+\+|---)( |$)/.test(row)) return
        if (row.startsWith('+')) this.addClassToHast(node, 'diff add')
        else if (row.startsWith('-')) this.addClassToHast(node, 'diff remove')
      },
    })
  }
  // `highlight="3, 5-7"` (digits, commas, optional spaces, and ranges) becomes shiki's meta string `{3,5-7}`
  const normalized = highlight
    .split(',')
    .map((p) => p.trim())
    .join(',')
  const meta = /^\d+(-\d+)?(,\d+(-\d+)?)*$/.test(normalized) ? { __raw: `{${normalized}}` } : undefined
  return highlighter.codeToHtml(source, { lang: l, ...dualTheme, transformers, meta })
}
