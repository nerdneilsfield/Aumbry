import { computed, watch } from 'vue'
import { useColorMode } from '@vueuse/core'
import { usePrinting } from '@/composables/usePrint'

// Dark is the house style; light is opt-in. Persisted in localStorage (silently skipped when unavailable).
const mode = useColorMode({
  attribute: 'class',
  modes: { light: 'light', dark: 'dark' },
  initialValue: 'dark',
  storageKey: 'aumbry-color-mode',
})

// Paper is white: while printing, the page, Mermaid and the charts are drawn light whatever theme was chosen (PRD 9.3).
// useColorMode owns the html class otherwise; this only swaps it for the duration of printing and puts it back.
const printing = usePrinting()
const isDark = computed(() => mode.value !== 'light' && !printing.value)
watch(isDark, (dark) => {
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.classList.toggle('light', !dark)
}, { flush: 'sync' })

export function useTheme() {
  const toggle = () => (mode.value = mode.value === 'light' ? 'dark' : 'light')
  return { isDark, toggle }
}
