import { ofetch } from 'ofetch'
import type {
  AdminSearchParams,
  AdminTree,
  DirPatch,
  DirSearchParams,
  DirTokenCreated,
  DirTokenRotated,
  DirTokensResponse,
  Directory,
  ItemDetail,
  ItemRevisionsResponse,
  SearchPage,
  ServerStatus,
  ServerVersion,
} from './types'

export interface ErrorBody {
  error?: string
  message?: string
  locked?: boolean
  dir_name?: string
}

/** Thrown for every non-2xx response; `body` is the parsed error payload. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly body: ErrorBody,
  ) {
    super(body.message || body.error || `请求失败 (${status})`)
  }
}

export const errorMessage = (e: unknown) => (e instanceof Error ? e.message : '未知错误')

// ofetch handles JSON bodies, 204/empty responses and content-type based parsing: application/xml comes back
// as text on success, application/json error bodies as objects (so no forced responseType anywhere).
const http = ofetch.create({
  baseURL: '/api/v1',
  credentials: 'same-origin',
  retry: false,
  onResponseError({ response }) {
    const data: unknown = response._data
    throw new ApiError(response.status, data && typeof data === 'object' ? (data as ErrorBody) : { message: String(data ?? '').slice(0, 200) })
  },
})

const enc = encodeURIComponent

export const api = {
  version: () => http<ServerVersion>('/version'),
  dir: (id: string) => http<Directory>(`/dirs/${enc(id)}`),
  unlock: (id: string, password: string) => http(`/dirs/${enc(id)}/unlock`, { method: 'POST', body: { password } }),
  searchDir: (id: string, p: DirSearchParams, limit: number, offset: number) =>
    http<SearchPage>(`/dirs/${enc(id)}/search`, { query: { q: p.q, type: p.type || undefined, tag: p.tag || undefined, limit, offset } }),
  /** `fromDir` is the directory the reader came from; the server uses it to pick which mount the neighbors come from. */
  item: (id: string, fromDir?: string) => http<ItemDetail>(`/items/${enc(id)}`, { query: { dir: fromDir || undefined } }),
  itemContent: (id: string) => http<string, 'text'>(`/items/${enc(id)}/content`),
  itemRevisions: (id: string) => http<ItemRevisionsResponse>(`/items/${enc(id)}/revisions`),
  itemRevisionContent: (id: string, rev: number) => http<string, 'text'>(`/items/${enc(id)}/revisions/${rev}/content`),

  admin: {
    login: (username: string, password: string) => http('/admin/login', { method: 'POST', body: { username, password } }),
    logout: () => http('/admin/logout', { method: 'POST' }),
    // 401 means "not logged in", an answer rather than a failure: an errored query is refetched whenever an observer mounts,
    // and the login form mounting on every failure made /admin loop between loading and the form.
    me: () =>
      http<{ admin: boolean }>('/admin/me').catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 401) return { admin: false }
        throw e
      }),
    status: () => http<ServerStatus>('/admin/status'),
    tree: () => http<AdminTree>('/admin/tree'),
    createDir: (body: { parent_id: string | null; name: string; description?: string; password?: string }) =>
      http<{ id: string }>('/admin/dirs', { method: 'POST', body }),
    patchDir: (id: string, patch: DirPatch) => http(`/admin/dirs/${enc(id)}`, { method: 'PATCH', body: patch }),
    deleteDir: (id: string) => http(`/admin/dirs/${enc(id)}`, { method: 'DELETE' }),
    setMounts: (id: string, dirs: string[]) => http(`/admin/items/${enc(id)}/mounts`, { method: 'PUT', body: { dirs } }),
    deleteItem: (id: string) => http(`/admin/items/${enc(id)}`, { method: 'DELETE' }),
    deleteRevision: (id: string, rev: number) => http(`/admin/items/${enc(id)}/revisions/${rev}`, { method: 'DELETE' }),
    listDirTokens: (dirId: string) => http<DirTokensResponse>(`/admin/dirs/${enc(dirId)}/tokens`),
    createDirToken: (dirId: string, label: string) =>
      http<DirTokenCreated>(`/admin/dirs/${enc(dirId)}/tokens`, { method: 'POST', body: { label } }),
    rotateDirToken: (dirId: string, tokenId: string) =>
      http<DirTokenRotated>(`/admin/dirs/${enc(dirId)}/tokens/${enc(tokenId)}/rotate`, { method: 'POST' }),
    deleteDirToken: (dirId: string, tokenId: string) =>
      http(`/admin/dirs/${enc(dirId)}/tokens/${enc(tokenId)}`, { method: 'DELETE' }),
    search: (p: AdminSearchParams, limit: number, offset: number) =>
      http<SearchPage>('/admin/search', {
        query: { q: p.q, type: p.type || undefined, from: p.from || undefined, to: p.to || undefined, dir: p.dir || undefined, unmounted: p.unmounted ? 1 : undefined, limit, offset },
      }),
  },
}
