import assert from 'node:assert/strict'
import test from 'node:test'
import { parseAnchorTarget } from '../src/lib/anchor.ts'
import { formatAttachmentExportMarkdown, pdfDocumentOptions } from '../src/lib/pdf.ts'
import { diffXmlDocuments } from '../src/lib/revisionDiff.ts'

test('parseAnchorTarget supports #att-<id>:p=<N> page stripping and rejects invalid forms', () => {
  assert.deepEqual(parseAnchorTarget('#att-paper:p=12'), { base: '#att-paper', page: 12 })
  assert.deepEqual(parseAnchorTarget('#att-paper'), { base: '#att-paper' })
  assert.deepEqual(parseAnchorTarget('#s-intro'), { base: '#s-intro' })

  assert.equal(parseAnchorTarget('#att-paper:p=0'), null)
  assert.equal(parseAnchorTarget('#att-paper:p=-2'), null)
  assert.equal(parseAnchorTarget('#att-paper:p=abc'), null)
  assert.equal(parseAnchorTarget('#s-intro:p=3'), null)
  assert.equal(parseAnchorTarget('#fig-a:p=1'), null)
})

test('pdfDocumentOptions enforces on-demand range loading and disables eval/prefetch', () => {
  const opts = pdfDocumentOptions('/api/v1/assets/abc')
  assert.deepEqual(opts, {
    url: '/api/v1/assets/abc',
    withCredentials: true,
    disableAutoFetch: true,
    disableStream: true,
    isEvalSupported: false,
  })
  assert.match(formatAttachmentExportMarkdown('Full Paper', '/api/v1/assets/abc'), /PDF 附件：Full Paper/)
})

test('revisionDiff detects PDF attachment additions and src/title changes', () => {
  const wrap = (body) =>
    `<document version="1.2"><meta><title>Doc</title><type>report</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta><body>${body}</body></document>`

  const oldXml = wrap(`<attachment id="p1" type="pdf" src="/api/v1/assets/111" title="Paper v1"/>`)
  const newXml = wrap(`<attachment id="p1" type="pdf" src="/api/v1/assets/222" title="Paper v2"/>`)

  const diff = diffXmlDocuments(oldXml, newXml)
  assert.equal(diff.stats.modified, 1)
  assert.equal(diff.blockChanges[0].tag, 'attachment')
  assert.equal(diff.blockChanges[0].anchorId, 'att-p1')
  assert.ok(diff.blockChanges[0].parts.some((p) => p.removed && p.value.includes('111')))
  assert.ok(diff.blockChanges[0].parts.some((p) => p.added && p.value.includes('222')))
})
