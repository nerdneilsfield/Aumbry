import type { Ref } from 'vue'
import { isFocusedElementEditable, onKeyStroke } from '@vueuse/core'

// Open dialogs, menus and select lists own the keyboard: `/` there must not move focus out from under them.
const OVERLAYS = '[role="dialog"], [role="alertdialog"], [role="listbox"], [role="menu"], [role="combobox"]'

/** Pressing `/` focuses the page's search box, unless the user is already typing somewhere or an overlay is open. */
export function useSearchShortcut(input: Readonly<Ref<HTMLInputElement | null | undefined>>) {
  onKeyStroke('/', (e) => {
    const el = input.value
    if (!el || e.ctrlKey || e.metaKey || e.altKey || isFocusedElementEditable()) return
    if (e.target instanceof Element && e.target.closest(OVERLAYS)) return
    e.preventDefault()
    el.focus()
  })
}
