/** A fenced Markdown code block; the fence is longer than any backtick run inside, so the content cannot close it early. */
export function fence(lang: string, source: string): string {
  const longest = Math.max(2, ...(source.match(/`+/g) ?? []).map((run) => run.length))
  const ticks = '`'.repeat(longest + 1)
  return `${ticks}${lang}\n${source.replace(/\n+$/, '')}\n${ticks}`
}
