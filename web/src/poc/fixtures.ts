import paperReading from './fixtures/paper-reading.xml?raw'
import dailyReport from './fixtures/daily-report.xml?raw'
import repoTeardown from './fixtures/repo-teardown.xml?raw'
import kitchenSink from './fixtures/kitchen-sink.xml?raw'

export interface Fixture {
  name: string
  label: string
  hint: string
  xml: string
}

// Bundled with ?raw so the demo is one self-contained file: nothing is fetched at runtime.
export const FIXTURES: Fixture[] = [
  { name: 'paper-reading', label: '论文精读', hint: '三轮精读：claim / equation / mermaid / chart / compare-table', xml: paperReading },
  { name: 'daily-report', label: '日报', hint: 'metrics / paper-card / repo-card / timeline', xml: dailyReport },
  { name: 'repo-teardown', label: '项目拆解', hint: 'mermaid 架构 / code-block diff / tabs / checklist', xml: repoTeardown },
  { name: 'kitchen-sink', label: '全标签与边界', hint: '每个标签 + 语法错误 / 非法 LaTeX / 恶意字符串', xml: kitchenSink },
]

// <figure src="./imgs/x.svg"> is rewritten to the inlined picture, the way the server rewrites it to /api/assets/{hash}.
const svgs = import.meta.glob<string>('./imgs/*.svg', { query: '?raw', import: 'default', eager: true })
const toDataUri = (svg: string) => {
  const bytes = new TextEncoder().encode(svg)
  let bin = ''
  bytes.forEach((b) => (bin += String.fromCharCode(b)))
  return `data:image/svg+xml;base64,${btoa(bin)}`
}
const images = new Map(Object.entries(svgs).map(([path, svg]) => [path.replace('./imgs/', ''), toDataUri(svg)]))

export function rewriteImages(body: Element) {
  body.querySelectorAll('figure[src]').forEach((f) => {
    const m = /^\.\/imgs\/([\w.-]+)$/.exec(f.getAttribute('src') ?? '')
    const uri = m && images.get(m[1])
    if (uri) f.setAttribute('src', uri)
  })
}
