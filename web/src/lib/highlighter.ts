import { createHighlighterCore } from 'shiki/core'
import { createJavaScriptRegexEngine } from 'shiki/engine/javascript'
import githubDark from 'shiki/themes/github-dark.mjs'
import githubLight from 'shiki/themes/github-light.mjs'
import type { CodeToHastOptions } from 'shiki/core'

// Top-level await: only the lazily loaded reader chunk imports this module.
export const highlighter = await createHighlighterCore({
  themes: [githubLight, githubDark],
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

export const loadedLangs = new Set(highlighter.getLoadedLanguages())

/** Light/dark dual theme: both colour sets are emitted as CSS variables, the html class picks one (see style.css). */
export const dualTheme = {
  themes: { light: 'github-light', dark: 'github-dark' },
  defaultColor: false,
} satisfies Partial<CodeToHastOptions>
