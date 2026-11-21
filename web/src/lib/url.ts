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
    // Resolved against the full page URL (not just its origin) so anchors also work on opaque origins such as file://.
    const page = new URL(window.location.href)
    return new URL(s, page).origin === page.origin ? s : undefined
  } catch {
    return undefined
  }
}

let dataImages = false
/** Offline single-file demo only: lets <figure> use the inlined `data:image/...;base64,` pictures it ships with. */
export const allowInlineDataImages = () => {
  dataImages = true
}
const INLINE_IMAGE = /^data:image\/(svg\+xml|png|jpeg|webp|gif);base64,[A-Za-z0-9+/=]+$/

/** Image sources: same-origin paths or http(s) URLs (plus inlined data images when explicitly enabled by the offline build). */
export const imageSrc = (raw: string | null | undefined) => {
  const s = raw?.trim()
  return dataImages && s && INLINE_IMAGE.test(s) ? s : safeUrl(raw)
}

export const itemPath = (item: { id: string; type: string }) =>
  `/${item.type === 'post' ? 'p' : 'r'}/${encodeURIComponent(item.id)}`
