export type ItemType = 'report' | 'post'

export interface ItemSummary {
  id: string
  type: ItemType
  title: string
  date: string
  summary: string
  tags: string[]
  source_agent: string
}

/** snippet is server-escaped HTML containing only <mark> tags. */
export interface SearchResult extends ItemSummary {
  snippet: string
}

export interface SearchPage {
  results: SearchResult[]
  limit: number
  offset: number
}

export interface DirRef {
  id: string
  name: string
}

export interface DirChild extends DirRef {
  description: string
  locked: boolean
}

export interface Directory extends DirRef {
  description: string
  parent_id: string | null
  breadcrumb: DirRef[]
  children: DirChild[]
  items: ItemSummary[]
}

export interface ItemDetail extends ItemSummary {
  canonical_url: string
  created_at: string
  updated_at: string
  dirs: DirRef[]
  related: ItemSummary[]
}

export interface DirNode extends DirRef {
  description: string
  has_password: boolean
  item_count: number
  sort_order: number
  children: DirNode[]
}

export interface AdminTree {
  tree: DirNode[]
  unmounted_count: number
}

export interface AdminSearchParams {
  q?: string
  type?: ItemType | ''
  from?: string
  to?: string
  dir?: string | null
  unmounted?: boolean
}

export interface DirPatch {
  name?: string
  description?: string
  parent_id?: string | null
  password?: string
  sort_order?: number
}
