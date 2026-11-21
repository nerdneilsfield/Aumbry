import type { InjectionKey } from 'vue'

/**
 * How a <news-item detail-id="…"> opens its detail entry. The real reader falls back to the router (`/p/:id`); the offline
 * demo provides its own function that switches to the bundled fixture with that id.
 */
export const DETAIL_NAV: InjectionKey<(id: string) => void> = Symbol('aumbry-detail-nav')
