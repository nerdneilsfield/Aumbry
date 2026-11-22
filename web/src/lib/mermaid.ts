import DOMPurify from 'dompurify'

// mermaid is a multi-megabyte dependency: it is only ever reached through this dynamic import, so documents
// without <mermaid> never download it. (No static `import mermaid` anywhere else!)
type Mermaid = (typeof import('mermaid'))['default']
let loading: Promise<Mermaid> | undefined
let configuredFor: 'dark' | 'light' | undefined
let queue: Promise<unknown> = Promise.resolve()
let seq = 0

const FONT = 'Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", "PingFang SC", "Noto Sans SC", sans-serif'

const THEMES = {
  dark: {
    background: '#111827',
    primaryColor: '#1f2937',
    primaryTextColor: '#f3f4f6',
    primaryBorderColor: '#60a5fa',
    secondaryColor: '#1e293b',
    secondaryTextColor: '#f3f4f6',
    secondaryBorderColor: '#a78bfa',
    tertiaryColor: '#111827',
    tertiaryTextColor: '#e5e7eb',
    lineColor: '#9ca3af',
    textColor: '#e5e7eb',
    noteBkgColor: '#374151',
    noteTextColor: '#f9fafb',
    edgeLabelBackground: '#111827',
    clusterBkg: '#111827',
    clusterBorder: '#4b5563',
  },
  light: {
    background: '#ffffff',
    primaryColor: '#eff6ff',
    primaryTextColor: '#111827',
    primaryBorderColor: '#3b82f6',
    secondaryColor: '#f5f3ff',
    secondaryTextColor: '#111827',
    secondaryBorderColor: '#8b5cf6',
    tertiaryColor: '#f9fafb',
    tertiaryTextColor: '#1f2937',
    lineColor: '#6b7280',
    textColor: '#1f2937',
    noteBkgColor: '#fef9c3',
    noteTextColor: '#1f2937',
    edgeLabelBackground: '#ffffff',
    clusterBkg: '#f9fafb',
    clusterBorder: '#d1d5db',
  },
}

/**
 * Renders Mermaid source to a sanitised SVG string. Rejects when the source does not parse.
 * securityLevel 'strict' (labels are encoded, no click handlers), plus a second DOMPurify pass with the SVG profile.
 * Renders are serialised: mermaid keeps global state while drawing.
 */
export function renderMermaid(source: string, dark: boolean): Promise<string> {
  const job = queue.then(async () => {
    const mermaid = await (loading ??= import('mermaid').then((m) => m.default))
    const theme = dark ? 'dark' : 'light'
    if (configuredFor !== theme) {
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        suppressErrorRendering: true, // we show our own fallback; mermaid's error SVG would be appended to <body>
        theme: 'base',
        themeVariables: { ...THEMES[theme], fontFamily: FONT },
        // SVG <text> labels instead of <foreignObject> HTML: survives the SVG-profile sanitiser untouched.
        htmlLabels: false,
        flowchart: { htmlLabels: false, useMaxWidth: true, curve: 'basis' },
        sequence: { useMaxWidth: true },
      })
      configuredFor = theme
    }
    const { svg } = await mermaid.render(`aumbry-mmd-${++seq}`, source)
    return DOMPurify.sanitize(svg, { USE_PROFILES: { svg: true, svgFilters: true } })
  })
  queue = job.catch(() => undefined)
  return job
}
