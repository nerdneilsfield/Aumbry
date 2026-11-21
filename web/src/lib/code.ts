import type { ShikiTransformer } from 'shiki/core'
import { transformerNotationDiff, transformerNotationHighlight } from '@shikijs/transformers'
import { dualTheme, highlighter, loadedLangs } from '@/lib/highlighter'

type LineKind = 'add' | 'remove' | undefined

/** Unified-diff text -> plain code (markers stripped, so the language grammar sees real code) + per-line kinds. */
function splitDiff(code: string): { code: string; kinds: LineKind[] } {
  const kinds: LineKind[] = []
  const out = code.split('\n').map((l) => {
    if (/^(\+\+\+|---) (a\/|b\/|\/dev\/null)|^(@@|diff )/.test(l)) {
      kinds.push(undefined)
      return l
    }
    const c = l[0]
    kinds.push(c === '+' ? 'add' : c === '-' ? 'remove' : undefined)
    return c === '+' || c === '-' || c === ' ' ? l.slice(1) : l
  })
  return { code: out.join('\n'), kinds }
}

/**
 * Highlights code with shiki (light+dark dual theme) and @shikijs/transformers: `// [!code highlight]` marks a line,
 * `// [!code ++]` / `// [!code --]` mark diff lines; with diff=true, leading +/- of a unified diff do the same.
 * Returns HTML produced by shiki, which escapes the code itself.
 */
export function highlightCode(source: string, lang: string, diff: boolean): string {
  // a unified diff's own grammar would colour whole lines as add/remove: highlight the stripped code as markdown instead
  const l = diff && lang === 'diff' ? 'markdown' : loadedLangs.has(lang) ? lang : 'text'
  const sp = diff ? splitDiff(source) : { code: source, kinds: [] as LineKind[] }
  const transformers: ShikiTransformer[] = [transformerNotationDiff(), transformerNotationHighlight()]
  if (diff) {
    transformers.push({
      name: 'aumbry-unified-diff',
      pre(node) {
        this.addClassToHast(node, 'has-diff')
      },
      line(node, line) {
        const k = sp.kinds[line - 1]
        if (k) this.addClassToHast(node, `diff ${k}`)
      },
    })
  }
  return highlighter.codeToHtml(sp.code, { lang: l, ...dualTheme, transformers })
}
