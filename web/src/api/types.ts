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
  protected: boolean
  parent_id: string | null
  breadcrumb: DirRef[]
  children: DirChild[]
  items: ItemSummary[]
}

export interface ReferencingItem extends ItemSummary {
  link_kinds: string[]
}

/** Previous (earlier) and next (later) item in one directory the caller may open; either is null at the ends. */
export interface Neighbors {
  dir: DirRef
  prev: ItemSummary | null
  next: ItemSummary | null
}

export interface ItemDetail extends ItemSummary {
  canonical_url: string
  created_at: string
  updated_at: string
  dirs: DirRef[]
  related: ItemSummary[]
  referenced_by: ReferencingItem[]
  revision_count: number
  current_rev: number
  /** null when the item has no mount the caller can open (e.g. an unmounted item seen by the admin). */
  neighbors: Neighbors | null
}

export interface ItemRevisionSummary {
  rev: number
  title: string
  summary: string
  date: string
  created_at: string
  is_current: boolean
}

export interface ItemRevisionsResponse {
  item_id: string
  current_rev: number
  revisions: ItemRevisionSummary[]
}

export interface DirTokenInfo {
  id: string
  directory_id: string
  label: string
  created_at: string
}

export interface DirTokensResponse {
  directory_id: string
  tokens: DirTokenInfo[]
}

export interface DirTokenCreated extends DirTokenInfo {
  token: string
}

export interface DirTokenRotated {
  id: string
  directory_id: string
  token: string
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

/** Filters of a directory-scoped search; empty strings mean "no filter". */
export interface DirSearchParams {
  q: string
  type: ItemType | ''
  tag: string
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

export interface ServerVersion {
  version: string
  commit: string
  build_time: string
}

export interface RuntimeStatus {
  go_version: string
  goos: string
  goarch: string
  num_cpu: number
  goroutines: number
  uptime_seconds: number
  started_at: string
  cpu_seconds_total: number
  cpu_percent: number
  heap_alloc_bytes: number
  heap_inuse_bytes: number
  sys_bytes: number
  gc_count: number
  process_rss_bytes?: number
}

export interface StorageStatus {
  backend: string
  sqlite_bytes: number
}

export interface ContentCounts {
  directories: number
  protected_directories: number
  items: number
  published_items: number
  draft_items: number
  revisions: number
  assets: number
  asset_bytes: number
  directory_tokens: number
}

export interface ActivityStatus {
  ingest_in_flight: number
  ingest_limit: number
  mcp_active_sessions: number
  mcp_anon_sessions: number
  mcp_anon_posts_in_flight: number
}

export interface ConfigStatus {
  max_ingest_body_bytes: number
  max_revisions_per_item: number
  allow_remote_images: boolean
  public_url_configured: boolean
  trusted_proxies_count: number
}

export interface ServerStatus {
  status: 'ok' | 'degraded' | string
  checks: Record<string, string>
  build: ServerVersion
  runtime: RuntimeStatus
  storage: StorageStatus
  counts: ContentCounts
  activity: ActivityStatus
  config: ConfigStatus
}
