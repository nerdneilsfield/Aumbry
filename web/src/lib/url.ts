/** Returns the URL only if it is an absolute http(s) URL, otherwise undefined. */
export function httpUrl(raw: string | null | undefined): string | undefined {
  if (!raw) return undefined
  try {
    const u = new URL(raw.trim())
    return u.protocol === 'http:' || u.protocol === 'https:' ? u.href : undefined
  } catch {
    return undefined
  }
}

/** Image sources: same-origin asset paths or http(s) URLs. */
export function imageSrc(raw: string | null | undefined): string | undefined {
  const s = raw?.trim()
  if (s && s.startsWith('/') && !s.startsWith('//')) return s
  return httpUrl(s)
}

export const itemPath = (item: { id: string; type: string }) =>
  `/${item.type === 'post' ? 'p' : 'r'}/${encodeURIComponent(item.id)}`
