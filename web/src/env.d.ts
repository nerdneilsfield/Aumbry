declare module 'markdown-it-texmath' {
  import type { PluginWithOptions } from 'markdown-it'
  const texmath: PluginWithOptions<{
    engine: unknown
    delimiters?: string | string[]
    katexOptions?: Record<string, unknown>
  }>
  export default texmath
}
