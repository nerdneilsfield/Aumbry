import { computed, nextTick, shallowRef } from 'vue'
import { useEventListener, useMediaQuery } from '@vueuse/core'

/**
 * Print state (PRD 9.3, 6.7, 9.7), one shared module-level state (one media-query listener, not one per block).
 *
 * `printing`: the page is going to paper. True while the browser's print media applies (print preview, `emulateMedia print`)
 * and between `beforeprint` and `afterprint` or while `printDocument()` runs. It switches the theme to light (useTheme) and makes
 * the lazily drawn figures (Mermaid, charts, pictures) draw in full.
 * `expanded`: everything that is hidden on screen (collapse, tabs, locked stages, steps, skim folds, news detail, search log) is
 * mounted and shown. True whenever `printing` is, and while a Markdown export reads the page.
 *
 * `beforeprint` alone cannot wait for async work (Mermaid, the chart chunk), so `printDocument()` draws first and only then
 * calls `window.print()`.
 */
const media = useMediaQuery('print')
const printHolds = shallowRef(0)
const exportHolds = shallowRef(0)
const printing = computed(() => media.value || printHolds.value > 0)
const expanded = computed(() => printing.value || exportHolds.value > 0)

/** Counts one more holder in `holds` until the returned release function is called (idempotent). */
function hold(holds: typeof printHolds): () => void {
  holds.value++
  let released = false
  return () => {
    if (!released) holds.value--
    released = true
  }
}

let printHold: (() => void) | undefined
const beginPrint = () => (printHold ??= hold(printHolds))
const endPrint = () => {
  printHold?.()
  printHold = undefined
}
useEventListener(window, 'beforeprint', beginPrint)
useEventListener(window, 'afterprint', endPrint)

// Renders that are still running (Mermaid drawing, a chart waiting for its chunk and its animation). Blocks register
// themselves with trackRender() so that printing can wait for them.
const rendering = new Set<Promise<unknown>>()

export function trackRender<T>(job: Promise<T>): Promise<T> {
  rendering.add(job)
  const done = () => void rendering.delete(job)
  job.then(done, done)
  return job
}

const SETTLE_TIMEOUT_MS = 15_000
const frame = () => new Promise<void>((r) => requestAnimationFrame(() => r()))

/**
 * Waits until the expanded page has been mounted and every tracked render has finished. Mounting content starts new renders (a
 * tab opens, its chart loads), so it only returns after two quiet frames in a row; a render that never finishes (a failed chunk)
 * cannot hold it longer than SETTLE_TIMEOUT_MS.
 */
async function settle() {
  const end = performance.now() + SETTLE_TIMEOUT_MS
  for (let quiet = 0; quiet < 2 && performance.now() < end; ) {
    await nextTick()
    await frame()
    if (rendering.size === 0) {
      quiet++
      continue
    }
    quiet = 0
    await Promise.race([Promise.allSettled([...rendering]), new Promise((r) => setTimeout(r, Math.max(end - performance.now(), 0)))])
  }
}

/** Expands everything, waits for the lazy figures, then opens the browser's print dialog (`afterprint` collapses it again). */
export async function printDocument() {
  beginPrint()
  try {
    await settle()
    window.print()
  } catch (e) {
    endPrint() // no afterprint will follow a print() that threw
    throw e
  }
}

/** Runs `fn` while the whole document is expanded and mounted (no waiting for Mermaid / charts), then puts the page back. */
export async function withExpandedDocument<T>(fn: () => T): Promise<T> {
  const release = hold(exportHolds)
  try {
    await nextTick()
    await frame()
    return fn()
  } finally {
    release()
  }
}

export const usePrinting = () => printing
export const useExpanded = () => expanded
