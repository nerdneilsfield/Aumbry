import assert from 'node:assert/strict'
import test from 'node:test'
import { parseAnchorTarget } from '../src/lib/anchor.ts'
import { formatBytes, formatCpuPercent, formatUptime } from '../src/lib/format.ts'
import { formatAttachmentExportMarkdown, pdfDocumentOptions } from '../src/lib/pdf.ts'
import { diffXmlDocuments } from '../src/lib/revisionDiff.ts'
import { formatBuildTime, formatVersionSummary, formatVersionTag } from '../src/lib/version.ts'

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

test('version helpers format numeric versions, short commits, and build timestamps', () => {
  assert.equal(formatVersionTag('0.2.0'), 'v0.2.0')
  assert.equal(formatVersionTag('v0.2.0'), 'v0.2.0')
  assert.equal(formatVersionTag('dev'), 'dev')
  assert.equal(formatVersionTag(' '), 'dev')

  assert.equal(formatBuildTime('2026-11-23T12:18:00Z'), '2026-11-23 12:18 UTC')
  assert.equal(formatBuildTime('unknown'), 'unknown')

  assert.deepEqual(
    formatVersionSummary({ version: '0.2.0', commit: '5ba6832', build_time: '2026-11-23T12:18:00Z' }),
    {
      version: 'v0.2.0',
      commit: '5ba6832',
      buildTime: '2026-11-23 12:18 UTC',
      rawBuildTime: '2026-11-23T12:18:00Z',
    },
  )
})

test('status format helpers render bytes, uptime, and cpu percentages cleanly', () => {
  assert.equal(formatBytes(0), '0 B')
  assert.equal(formatBytes(-5), '0 B')
  assert.equal(formatBytes(Number.NaN), '0 B')
  assert.equal(formatBytes(512), '512 B')
  assert.equal(formatBytes(1536), '1.50 KB')
  assert.equal(formatBytes(Math.round(12.3 * 1024 * 1024)), '12.3 MB')
  assert.equal(formatBytes(Math.round(2.4 * 1024 * 1024 * 1024)), '2.40 GB')

  assert.equal(formatUptime(-1), '0s')
  assert.equal(formatUptime(45), '45s')
  assert.equal(formatUptime(12 * 60 + 30), '12m 30s')
  assert.equal(formatUptime(3 * 3600 + 15 * 60), '3h 15m')
  assert.equal(formatUptime(2 * 86400 + 4 * 3600 + 10 * 60), '2d 4h 10m')

  assert.equal(formatCpuPercent(-1), '0.0%')
  assert.equal(formatCpuPercent(Number.NaN), '0.0%')
  assert.equal(formatCpuPercent(12.54), '12.5%')
})
