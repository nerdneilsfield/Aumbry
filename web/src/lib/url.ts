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

/**
 * Returns a URL that is safe to put in href/src, otherwise undefined: an absolute http(s) URL, or a
 * relative/anchor URL that the browser resolves to this very origin. Decided by the WHATWG URL parser, so
 * the check sees what the browser will see (`//evil.com`, `/\evil.com`, `/<TAB>/evil.com` all resolve cross-origin).
 */
export function safeUrl(raw: string | null | undefined): string | undefined {
  const s = raw?.trim()
  if (!s || s.includes('\\')) return undefined
  if (URL.canParse(s)) return httpUrl(s)
  try {
    return new URL(s, window.location.origin).origin === window.location.origin ? s : undefined
  } catch {
    return undefined
  }
}

/** Image sources: same-origin paths or http(s) URLs. */
export const imageSrc = safeUrl

export const itemPath = (item: { id: string; type: string }) =>
  `/${item.type === 'post' ? 'p' : 'r'}/${encodeURIComponent(item.id)}`
