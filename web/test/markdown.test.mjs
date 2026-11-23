import test from 'node:test'
import assert from 'node:assert/strict'
import { createMarkdownEngine, extractInlineText, renderInlineRaw, renderMarkdownRaw } from '../src/lib/markdownEngine.ts'

const md = createMarkdownEngine()
const render = (src, id = 't1') => renderMarkdownRaw(md, src, id)
const renderInline = (src) => renderInlineRaw(md, src)
const inlineText = (src) => extractInlineText(md, src)

const countMatches = (str, re) => (str.match(re) ?? []).length

test('renders inline \\(...\\) and paragraph-adjacent \\[...\\] repro from MATH-ISSUE', () => {
  const src = `一次推理输出未来 \\(H\\) 步动作，执行过时 \\(\\delta_{\\mathrm{exec}}=L\\)。

拆开之后：
\\[
a_{t:t+H}=\\pi(o_t),\\qquad \\delta_{\\mathrm{exec}}=L
\\]`
  const html = render(src)

  assert.equal(countMatches(html, /<eq><span data-tex=/g), 2)
  assert.equal(countMatches(html, /<section><eqn><span data-tex=/g), 1)
  assert.equal(countMatches(html, /class="katex-display"/g), 1)
  assert.equal(countMatches(html, /katex-error/g), 0)
  assert.match(html, /<eq><span data-tex="H">/)
  assert.match(html, /<eq><span data-tex="\\delta_\{\\mathrm\{exec\}\}=L">/)
  assert.match(html, /<p>拆开之后：<\/p>\s*<section><eqn><span data-tex="a_\{t:t\+H\}=\\pi\(o_t\),\\qquad \\delta_\{\\mathrm\{exec\}\}=L">/)
})

test('renders single-line \\[...\\] immediately after paragraph with no blank line', () => {
  const html = render('拆开之后：\n\\[ T_p \\ge T_u \\]')
  assert.equal(countMatches(html, /class="katex-display"/g), 1)
  assert.equal(countMatches(html, /katex-error/g), 0)
  assert.match(html, /<p>拆开之后：<\/p>\s*<section><eqn><span data-tex=" T_p \\ge T_u ">/)
})

test('renders \\[...\\] immediately after heading with no blank line', () => {
  const html = render('## 标题\n\\[\nT_p \\ge T_u\n\\]')
  assert.equal(countMatches(html, /class="katex-display"/g), 1)
  assert.equal(countMatches(html, /katex-error/g), 0)
  assert.match(html, /<h2>标题<\/h2>\s*<section><eqn>/)
})

test('renders standalone \\[...\\] with blank lines and optional equation number', () => {
  const plain = render('前文：\n\n\\[\nx^2 + y^2 = z^2\n\\]\n\n后文。')
  assert.equal(countMatches(plain, /class="katex-display"/g), 1)
  assert.match(plain, /<p>前文：<\/p>\s*<section><eqn>/)
  assert.match(plain, /<\/section>\s*<p>后文。<\/p>/)

  const numbered = render('拆开之后：\n\\[ T_p \\ge T_u \\] (1)')
  assert.equal(countMatches(numbered, /class="katex-display"/g), 1)
  assert.match(numbered, /<section class="eqno"><eqn>.*<\/eqn><span>\(1\)<\/span><\/section>/)
})

test('does not consume parenthesized paragraph after blank line as equation number', () => {
  const html = render('\\[\na + b\n\\]\n\n(见附录 A) 后面还有文字。')
  assert.equal(countMatches(html, /class="eqno"/g), 0)
  assert.equal(countMatches(html, /class="katex-display"/g), 1)
  assert.match(html, /<p>\(见附录 A\) 后面还有文字。<\/p>/)
})

test('does not turn escaped brackets or confidence intervals into math', () => {
  const cases = [
    ['95% 置信区间为 \\[-1.4; +4.3\\]，显著优于基线。', '[-1.4; +4.3]', '显著优于基线'],
    ['\\[-1.4; +4.3\\] 是 95% 置信区间。', '[-1.4; +4.3] 是 95% 置信区间。', '是 95% 置信区间'],
    ['95% 置信区间为\n\\[-1.4; +4.3\\]，显著优于基线。', '[-1.4; +4.3]，显著优于基线。', '95% 置信区间为'],
    ['\\[-1.4; +4.3\\] 与 \\[0.2; 1.8\\]', '[-1.4; +4.3] 与 [0.2; 1.8]', '[0.2; 1.8]'],
  ]
  for (const [src, expectedA, expectedB] of cases) {
    const html = render(src)
    assert.equal(countMatches(html, /katex/g), 0, `unexpected katex for: ${src}`)
    assert.ok(html.includes(expectedA), `missing ${expectedA} in: ${html}`)
    assert.ok(html.includes(expectedB), `missing ${expectedB} in: ${html}`)
  }
})

test('preserves $...$ and $$...$$ regressions and supports mixing with bracket delimiters', () => {
  const html = render('行内 $a+b$ 与 \\(c+d\\)，块级：\n\n$$\ne^2\n$$\n紧接：\n\\[\nf^2\n\\]')
  assert.equal(countMatches(html, /<eq><span data-tex=/g), 2)
  assert.equal(countMatches(html, /<section><eqn><span data-tex=/g), 2)
  assert.equal(countMatches(html, /class="katex-display"/g), 2)
  assert.equal(countMatches(html, /katex-error/g), 0)

  const adjacentDollars = render('紧跟：\n$$a + b$$')
  assert.equal(countMatches(adjacentDollars, /class="katex-display"/g), 1)
})

test('supports aligned blocks with \\\\[4pt] line-break spacing inside \\[...\\]', () => {
  const html = render('\\[\n\\begin{aligned}\na &= b \\\\[4pt]\nc &= d\n\\end{aligned}\n\\]')
  assert.equal(countMatches(html, /class="katex-display"/g), 1)
  assert.equal(countMatches(html, /katex-error/g), 0)
  assert.match(html, /a &amp;= b \\\\\[4pt\]/)
})

test('supports blockquotes, lists, tables, indented CDATA dedent, and CRLF', () => {
  const bq = render('> 推导：\n> \\[\n> a + b\n> \\]\n> 结论。')
  assert.match(bq, /<blockquote>\s*<p>推导：<\/p>\s*<section><eqn>/)
  assert.match(bq, /<p>结论。<\/p>\s*<\/blockquote>/)

  const list = render('- 推导：\n  \\[\n  a + b\n  \\]\noutside')
  assert.match(list, /<ul>\s*<li>推导：<section><eqn>.*<\/section><\/li>\s*<\/ul>\s*<p>outside<\/p>/s)

  const table = render('| 变量 | 含义 |\n| :--- | :--- |\n| \\(\\delta_{\\mathrm{exec}}\\) | 执行过时 |')
  assert.equal(countMatches(table, /<eq><span data-tex="\\delta_\{\\mathrm\{exec\}\}">/g), 1)

  const indentedCdata = render('\n    \\(\n    H\n    \\)\n\n    \\[\n    a + b\n    \\]\n  ')
  assert.equal(countMatches(indentedCdata, /class="katex"/g), 2)
  assert.equal(countMatches(indentedCdata, /class="katex-display"/g), 1)

  const crlf = render('拆开之后：\r\n\\[\r\nT_p \\ge T_u\r\n\\]')
  assert.equal(countMatches(crlf, /class="katex-display"/g), 1)
  assert.equal(countMatches(crlf, /katex-error/g), 0)
})

test('leaves code spans, fenced blocks, indented blocks, and escaped backslashes untouched', () => {
  const codeSpan = render('代码 `\\(H\\)` 与 `\\[x\\]`')
  assert.equal(countMatches(codeSpan, /katex/g), 0)
  assert.match(codeSpan, /<code>\\\(H\\\)<\/code>/)
  assert.match(codeSpan, /<code>\\\[x\\\]<\/code>/)

  const fence = render('```latex\n\\[\na + b\n\\]\n```')
  assert.equal(countMatches(fence, /katex/g), 0)
  assert.match(fence, /<pre><code/)

  const indented = render('第一段\n第二段\n\n    \\[\n    a + b\n    \\]\n\n    \\(H\\)')
  assert.equal(countMatches(indented, /katex/g), 0)
  assert.match(indented, /<pre><code>/)

  const escapedBs = render('字面量 \\\\(H\\\\) 与\n\n\\\\[\nx\n\\\\]')
  assert.equal(countMatches(escapedBs, /katex/g), 0)
})

test('handles unclosed delimiters, link boundaries, control spaces, and $&/$1 safely', () => {
  const unclosedInline = render('前缀 \\( 没闭合，后面 \\(x+1\\) 正常。')
  assert.equal(countMatches(unclosedInline, /<eq>/g), 1)
  assert.match(unclosedInline, /<eq><span data-tex="x\+1">/)

  const unclosedBlock = render('\\[ 没闭合\n\n\\[\nx = 1\n\\]')
  assert.equal(countMatches(unclosedBlock, /class="katex-display"/g), 1)
  assert.match(unclosedBlock, /<p>\[ 没闭合<\/p>/)

  const unclosedBeforeFence = render('\\[ 没闭合\n\n```tex\n\\]\n```')
  assert.equal(countMatches(unclosedBeforeFence, /katex/g), 0)
  assert.match(unclosedBeforeFence, /<pre><code/)

  const mathInLink = render('[公式 \\(a+b\\)](https://example.com)')
  assert.match(mathInLink, /<a href="https:\/\/example\.com">.*<eq><span data-tex="a\+b">/)
  assert.equal(countMatches(mathInLink, /katex-error/g), 0)

  const unclosedInLink = render('[text \\(a](https://example.com)')
  assert.equal(countMatches(unclosedInLink, /katex/g), 0)
  assert.match(unclosedInLink, /<a href="https:\/\/example\.com">text \(a<\/a>/)

  const controlSpace = render('\\(x\\ \\)')
  assert.match(controlSpace, /<eq><span data-tex="x\\ ">/)

  const replacementTokens = render('\\(\\text{\\$&}\\) 与\n\\[\n\\text{\\$1}\n\\]')
  assert.equal(countMatches(replacementTokens, /<span data-tex=/g), 2)
  assert.equal(countMatches(replacementTokens, /<section><eqn>/g), 1)
  assert.match(replacementTokens, /data-tex="\\text\{\\\$&amp;\}"/)
  assert.match(replacementTokens, /data-tex="\\text\{\\\$1\}"/)
  assert.equal(countMatches(replacementTokens, /<eq>/g), 1)
})

test('renderInlineRaw and extractInlineText handle bracket math and escaped brackets', () => {
  const inlineMath = renderInline('未来 \\(H\\) 步')
  assert.equal(countMatches(inlineMath, /<eq><span data-tex="H">/g), 1)

  const inlineCi = renderInline('区间 \\[-1.4; +4.3\\]')
  assert.equal(countMatches(inlineCi, /katex/g), 0)
  assert.equal(inlineCi, '区间 [-1.4; +4.3]')

  assert.equal(inlineText('一次推理输出未来 \\(H\\) 步与 $L$ 步'), '一次推理输出未来 H 步与 L 步')
})
