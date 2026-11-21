import paperReading from './fixtures/paper-reading.xml?raw'
import reproReport from './fixtures/repro-report.xml?raw'
import learningPath from './fixtures/learning-path.xml?raw'
import newsDigest from './fixtures/news-digest.xml?raw'
import newsDetail from './fixtures/news-detail.xml?raw'
import weeklyReport from './fixtures/weekly-report.xml?raw'
import dailyReport from './fixtures/daily-report.xml?raw'
import repoTeardown from './fixtures/repo-teardown.xml?raw'
import freeForm from './fixtures/free-form.xml?raw'
import onePager from './fixtures/one-pager.xml?raw'
import multiStages from './fixtures/multi-stages.xml?raw'
import roundOne from './fixtures/round-one.xml?raw'
import kitchenSink from './fixtures/kitchen-sink.xml?raw'

export type Kind = '论文' | '复现' | '学习路径' | '新闻' | '周报' | '日报' | '项目' | '自由结构' | '边界'
export const KINDS: Kind[] = ['论文', '复现', '学习路径', '新闻', '周报', '日报', '项目', '自由结构', '边界']

export interface Fixture {
  name: string
  kind: Kind
  label: string
  hint: string
  xml: string
  /** the 12-character item id other fixtures use in `detail-id` / `related-ids` to point here (offline stand-in for the server's id) */
  id: string
  /** the document has at least one <stages> group */
  staged: boolean
  /** reachable through ?doc= only (test variants) */
  hidden?: boolean
  /** saved progress is kept under this name instead of `name` (a variant shares the original's progress) */
  storeAs?: string
}

// Variant of paper-reading with the first two options of quiz `q-nfe` swapped: same quiz id, different fingerprint.
const OPT_A = '<option>仍然约 30 FPS，采样步数不影响延迟</option>'
const OPT_B = '<option correct="true">约 3 FPS 量级：延迟近似与 NFE 成正比，主干没变</option>'
const swapped = paperReading.includes(OPT_A + '\n          ' + OPT_B) ? paperReading.replace(OPT_A + '\n          ' + OPT_B, OPT_B + '\n          ' + OPT_A) : paperReading

const fx = (f: Omit<Fixture, 'staged'>): Fixture => ({ ...f, staged: f.xml.includes('<stages') })

// Bundled with ?raw so the demo is one self-contained file: nothing is fetched at runtime.
export const FIXTURES: Fixture[] = [
  fx({ name: 'paper-reading', kind: '论文', label: '论文精读', id: 'Pap3rRead001', hint: '三轮精读，分三个阶段：每阶段有 tldr、测验、下一步选项；claim / equation / mermaid / chart / compare-table', xml: paperReading }),
  fx({ name: 'round-one', kind: '论文', label: '只读第一轮', id: 'RoundOne0001', hint: 'stages 里只有一个 stage：没有步进条、没有「继续」，阶段直接展开', xml: roundOne }),
  fx({ name: 'repro-report', kind: '复现', label: '复现报告', id: 'Repr0Report1', hint: '两个阶段，每阶段一个测验；未报告 / 近似单元格、误差棒图、公式↔代码芯片、检索记录', xml: reproReport }),
  fx({ name: 'learning-path', kind: '学习路径', label: 'BEV 学习路径', id: 'LearnPath001', hint: '入门 → 进阶 → 专家，每个阶段以测验收尾；glossary / mermaid / 交互式 steps', xml: learningPath }),
  fx({ name: 'news-digest', kind: '新闻', label: '新闻简报', id: 'NewsDigest01', hint: '按主题分节，每节一个 tldr；news-item（重要、内联详情、detail-id 跳转到详情条目）', xml: newsDigest }),
  fx({ name: 'news-detail', kind: '新闻', label: '新闻详情', id: 'NewsDetail01', hint: '一条新闻的独立详情：tldr、时间线、引述、据称未证实的 claim、sources；related-ids 指回简报', xml: newsDetail }),
  fx({ name: 'weekly-report', kind: '周报', label: '周报', id: 'Weekly26W470', hint: '按天分 section，每天一个 tldr，整篇再有一个全文 tldr；related-ids 指向各日报', xml: weeklyReport }),
  fx({ name: 'daily-report', kind: '日报', label: '日报', id: 'DailyRep0001', hint: 'metrics / paper-card / repo-card / timeline', xml: dailyReport }),
  fx({ name: 'repo-teardown', kind: '项目', label: '项目拆解', id: 'RepoTear0001', hint: 'mermaid 架构 / code-block diff / tabs / checklist', xml: repoTeardown }),
  fx({ name: 'free-form', kind: '自由结构', label: '自由结构', id: 'FreeForm0001', hint: 'body 直接放 claim 与 chart、自拟章节标题与顺序、3 层嵌套、tldr 只在一节里、没有分阶段', xml: freeForm }),
  fx({ name: 'multi-stages', kind: '自由结构', label: '多组分阶段', id: 'MultiStage01', hint: '4 组互相独立的 stages：body 开头一组、章节中间一组、专题自带一组（含测验）、gated="false" 一组；各自保存进度', xml: multiStages }),
  fx({ name: 'one-pager', kind: '自由结构', label: '一页结论', id: 'OnePager0001', hint: 'body 里只有一个 md 和两张图，没有 section、没有 tldr', xml: onePager }),
  fx({ name: 'paper-reading-swap', hidden: true, storeAs: 'paper-reading', kind: '论文', label: '论文精读（选项换了顺序）', id: 'Pap3rSwap001', hint: '测试变体：q-nfe 的选项顺序被调换，已保存的这道题的答案作废，阶段进度保留', xml: swapped }),
  fx({ name: 'kitchen-sink', kind: '边界', label: '全标签与边界', id: 'KitchenSink1', hint: '每个标签 + 语法错误 / 非法 LaTeX / 恶意字符串 / 各种无效的 quiz、tldr、news-item / 嵌套 stages / 4 层 section', xml: kitchenSink }),
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
