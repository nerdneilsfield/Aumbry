export const AUDIO_START_RE = /^(\d{2,}):([0-5]\d):([0-5]\d)$/

export interface AudioChapterMeta {
  start: string
  startSeconds: number
  title: string
  target?: string
}

/** Parses `HH:MM:SS` (`MM`/`SS` in 00..59) into seconds, or returns null when invalid. */
export function parseAudioTimestamp(raw: string): number | null {
  const m = AUDIO_START_RE.exec(raw.trim())
  if (!m) return null
  const hours = Number(m[1])
  const mins = Number(m[2])
  const secs = Number(m[3])
  if (!Number.isFinite(hours) || mins > 59 || secs > 59) return null
  return hours * 3600 + mins * 60 + secs
}

/** Returns the 0-based index of the active chapter for `currentTime`, or -1 before the first chapter. */
export function activeChapterIndex(chapters: readonly { startSeconds: number }[], currentTime: number): number {
  if (!Number.isFinite(currentTime) || currentTime < 0) return -1
  let active = -1
  for (let i = 0; i < chapters.length; i++) {
    if (currentTime >= chapters[i].startSeconds) {
      active = i
    }
  }
  return active
}

/** Formats an `<audio>` block as Markdown (title, chapter list, transcript) without player chrome. */
export function formatAudioExportMarkdown(
  title: string,
  chapters: readonly { start: string; title: string }[],
  transcript: string,
): string {
  const lines: string[] = [`### 音频：${title.trim() || '未命名音频'}`]
  if (chapters.length > 0) {
    lines.push('')
    for (const ch of chapters) {
      lines.push(`- \`${ch.start}\` ${ch.title}`)
    }
  }
  const cleanTranscript = transcript.trim()
  if (cleanTranscript) {
    lines.push('', '#### 逐字稿', '', cleanTranscript)
  }
  return lines.join('\n')
}
