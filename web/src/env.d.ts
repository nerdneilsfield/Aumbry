declare module 'markdown-it-texmath' {
  import type { PluginWithOptions } from 'markdown-it'
  const texmath: PluginWithOptions<{
    engine: unknown
    delimiters?: string | string[]
    katexOptions?: Record<string, unknown>
  }>
  export default texmath
}

declare module 'markdown-it-footnote' {
  import type { PluginSimple } from 'markdown-it'
  const footnote: PluginSimple
  export default footnote
}

declare module 'markdown-it-task-lists' {
  import type { PluginWithOptions } from 'markdown-it'
  const taskLists: PluginWithOptions<{ enabled?: boolean; label?: boolean; labelAfter?: boolean }>
  export default taskLists
}

declare module 'markdown-it-sub' {
  import type { PluginSimple } from 'markdown-it'
  const sub: PluginSimple
  export default sub
}

declare module 'markdown-it-sup' {
  import type { PluginSimple } from 'markdown-it'
  const sup: PluginSimple
  export default sup
}
