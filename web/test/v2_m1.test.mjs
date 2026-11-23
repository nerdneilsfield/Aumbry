import test from 'node:test'
import assert from 'node:assert/strict'
import { diffXmlDocuments } from '../src/lib/revisionDiff.ts'
import { activeChapterIndex, formatAudioExportMarkdown, parseAudioTimestamp } from '../src/lib/audio.ts'
import { createReadingStorage, stripQueryParam, syncQueryRecord } from '../src/lib/readingState.ts'

test('revisionDiff compares metadata, id-matched blocks across moves, explicit-id replacements, audio src swaps, and identical docs', () => {
  const v1 = `<?xml version="1.0" encoding="UTF-8"?>
<document version="1.2">
  <meta>
    <title>V1 Title</title>
    <type>report</type>
    <date>2026-11-22</date>
    <summary>Initial summary</summary>
    <source-agent>writer</source-agent>
    <tags><tag>ai</tag><tag>draft</tag></tags>
  </meta>
  <body>
    <section id="intro" title="Intro">
      <md><![CDATA[First paragraph unchanged.\nSecond line old.]]></md>
    </section>
    <audio id="pod" src="./a.mp3" title="Audio V1">
      <chapter start="00:00:00" title="Start" target="#s-intro"/>
    </audio>
    <figure id="arch" src="./old.png" caption="Old Caption"/>
  </body>
</document>`

  const v2 = `<?xml version="1.0" encoding="UTF-8"?>
<document version="1.2">
  <meta>
    <title>V2 Final Title</title>
    <type>report</type>
    <date>2026-11-23</date>
    <summary>Updated summary text</summary>
    <source-agent>writer</source-agent>
    <tags><tag>ai</tag><tag>podcast</tag></tags>
  </meta>
  <body>
    <figure id="arch" src="./old.png" caption="New Caption"/>
    <section id="intro" title="Intro">
      <md><![CDATA[First paragraph unchanged.\nSecond line revised.]]></md>
      <md><![CDATA[Newly added paragraph.]]></md>
    </section>
    <audio id="pod" src="./a.mp3" title="Audio V2">
      <chapter start="00:00:00" title="Start" target="#s-intro"/>
    </audio>
  </body>
</document>`

  const same = diffXmlDocuments(v1, v1)
  assert.equal(same.hasChanges, false)
  assert.equal(same.metaChanges.length, 0)
  assert.equal(same.blockChanges.length, 0)

  const diff = diffXmlDocuments(v1, v2)
  assert.equal(diff.hasChanges, true)
  assert.equal(diff.stats.metaChanged, 3)
  assert.deepEqual(
    diff.metaChanges.map((m) => m.field),
    ['title', 'summary', 'tags'],
  )

  const titleParts = diff.metaChanges.find((m) => m.field === 'title').parts
  assert.ok(titleParts.some((p) => p.removed && p.value.includes('V1')))
  assert.ok(titleParts.some((p) => p.added && p.value.includes('V2')))

  const tagParts = diff.metaChanges.find((m) => m.field === 'tags').parts
  assert.ok(tagParts.some((p) => p.removed && p.value === 'draft'))
  assert.ok(tagParts.some((p) => p.added && p.value === 'podcast'))

  // Reordered #fig-arch and #audio-pod still pair cleanly by shared `id` instead of added+removed.
  assert.equal(diff.stats.removed, 0)
  assert.equal(diff.stats.added, 1)
  assert.equal(diff.stats.modified, 3)

  const figChange = diff.blockChanges.find((c) => c.id === 'arch')
  assert.equal(figChange?.kind, 'modified')
  assert.equal(figChange?.anchorId, 'fig-arch')

  const audioChange = diff.blockChanges.find((c) => c.id === 'pod')
  assert.equal(audioChange?.kind, 'modified')
  assert.equal(audioChange?.anchorId, 'audio-pod')

  const mdMod = diff.blockChanges.find((c) => c.tag === 'md' && c.kind === 'modified')
  assert.ok(mdMod)
  assert.ok(mdMod.parts.some((p) => p.removed && p.value.includes('old')))
  assert.ok(mdMod.parts.some((p) => p.added && p.value.includes('revised')))

  const mdAdd = diff.blockChanges.find((c) => c.tag === 'md' && c.kind === 'added')
  assert.ok(mdAdd)

  // Audio src-only replacement (same title/chapters/transcript) MUST report modified.
  const audioSrc1 = `<document version="1.2"><meta><title>A</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta><body><audio id="ep" src="/api/v1/assets/1111" title="Same"/></body></document>`
  const audioSrc2 = `<document version="1.2"><meta><title>A</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta><body><audio id="ep" src="/api/v1/assets/2222" title="Same"/></body></document>`
  const audioSwap = diffXmlDocuments(audioSrc1, audioSrc2)
  assert.equal(audioSwap.hasChanges, true)
  assert.equal(audioSwap.stats.modified, 1)

  // Explicit id change (`id="a"` -> `id="b"`) with identical body content MUST report removed + added (not suppressed by ordinal pass).
  const idA = `<document version="1.2"><meta><title>A</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta><body><figure id="fig-a" src="/api/v1/assets/1111" caption="Same"/></body></document>`
  const idB = `<document version="1.2"><meta><title>A</title><type>post</type><date>2026-11-23</date><summary>S</summary><source-agent>a</source-agent></meta><body><figure id="fig-b" src="/api/v1/assets/1111" caption="Same"/></body></document>`
  const idDiff = diffXmlDocuments(idA, idB)
  assert.equal(idDiff.hasChanges, true)
  assert.equal(idDiff.stats.removed, 1)
  assert.equal(idDiff.stats.added, 1)
})

test('audio helpers parse timestamps strictly, select active chapter, and export markdown without audio tags', () => {
  assert.equal(parseAudioTimestamp('00:00:00'), 0)
  assert.equal(parseAudioTimestamp('01:02:03'), 3723)
  assert.equal(parseAudioTimestamp('100:59:59'), 363599)
  assert.equal(parseAudioTimestamp('00:60:00'), null)
  assert.equal(parseAudioTimestamp('00:00:60'), null)
  assert.equal(parseAudioTimestamp('1:02:03'), null)

  const chs = [
    { startSeconds: 0 },
    { startSeconds: 0 },
    { startSeconds: 45 },
    { startSeconds: 120 },
  ]
  assert.equal(activeChapterIndex(chs, -1), -1)
  assert.equal(activeChapterIndex(chs, 0), 1)
  assert.equal(activeChapterIndex(chs, 44.9), 1)
  assert.equal(activeChapterIndex(chs, 45), 2)
  assert.equal(activeChapterIndex(chs, 300), 3)

  const md = formatAudioExportMarkdown(
    'Weekly Pod',
    [
      { start: '00:00:00', title: 'Intro' },
      { start: '00:02:10', title: 'Main Topic' },
    ],
    'Hello listeners.\nWelcome.',
  )
  assert.match(md, /### 音频：Weekly Pod/)
  assert.match(md, /- `00:02:10` Main Topic/)
  assert.match(md, /Hello listeners\./)
  assert.equal(md.includes('<audio'), false)
})

test('readingState createReadingStorage in readOnly mode never reads or writes localStorage and preserves rev/dir params', () => {
  const backingData = new Map([
    ['aumbry:read:item123', JSON.stringify({ stages: { g1: 's2' }, quizzes: {} })],
    ['aumbry:skim:item123', 'true'],
  ])
  const touches = []
  const spyStorage = {
    getItem(k) {
      touches.push(`get:${k}`)
      return backingData.get(k) ?? null
    },
    setItem(k, v) {
      touches.push(`set:${k}`)
      backingData.set(k, v)
    },
    removeItem(k) {
      touches.push(`del:${k}`)
      backingData.delete(k)
    },
  }

  // Historical `?rev=N` view: createReadingStorage(true) must NEVER touch localStorage.
  const revStorage = createReadingStorage(true, spyStorage)
  assert.equal(revStorage.getItem('aumbry:read:item123'), null)
  assert.equal(revStorage.getItem('aumbry:skim:item123'), null)
  revStorage.setItem('aumbry:read:item123', JSON.stringify({ stages: { g1: 's1' }, quizzes: {} }))
  assert.match(revStorage.getItem('aumbry:read:item123'), /"s1"/)
  revStorage.setItem('aumbry:skim:item123', 'false')
  revStorage.removeItem('aumbry:skim:item123')
  assert.equal(touches.length, 0, `expected zero storage touches in readOnly mode, got ${touches.join(',')}`)
  assert.equal(revStorage.getItem('aumbry:skim:item123'), null)
  assert.equal(backingData.get('aumbry:skim:item123'), 'true')
  assert.match(backingData.get('aumbry:read:item123'), /"s2"/)

  // Live mode: createReadingStorage(false) delegates to backing storage for both keys.
  const liveStorage = createReadingStorage(false, spyStorage)
  assert.match(liveStorage.getItem('aumbry:read:item123'), /"s2"/)
  assert.equal(liveStorage.getItem('aumbry:skim:item123'), 'true')
  liveStorage.setItem('aumbry:skim:item123', 'false')
  assert.equal(backingData.get('aumbry:skim:item123'), 'false')

  // Query preservation across route sync and skim clearing.
  const params = {}
  syncQueryRecord('?dir=dir1&rev=3&skim=1', params)
  assert.deepEqual(params, { dir: 'dir1', rev: '3', skim: '1' })
  assert.equal(stripQueryParam('?dir=dir1&rev=3&skim=1', 'skim'), '?dir=dir1&rev=3')
})
