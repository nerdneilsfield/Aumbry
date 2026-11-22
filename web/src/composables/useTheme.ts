import { computed } from 'vue'
import { useColorMode } from '@vueuse/core'

// Dark is the house style; light is opt-in. Persisted in localStorage (silently skipped when unavailable).
const mode = useColorMode({
  attribute: 'class',
  modes: { light: 'light', dark: 'dark' },
  initialValue: 'dark',
  storageKey: 'aumbry-color-mode',
})

export function useTheme() {
  const isDark = computed(() => mode.value !== 'light')
  const toggle = () => (mode.value = isDark.value ? 'light' : 'dark')
  return { isDark, toggle }
}
