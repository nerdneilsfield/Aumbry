import { ofetch } from 'ofetch'
import type {
  AdminSearchParams,
  AdminTree,
  DirPatch,
  Directory,
  ItemDetail,
  SearchPage,
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

// ofetch handles JSON bodies, 204/empty responses and content-type based parsing (XML comes back as text).
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
  dir: (id: string) => http<Directory>(`/dirs/${enc(id)}`),
  unlock: (id: string, password: string) => http(`/dirs/${enc(id)}/unlock`, { method: 'POST', body: { password } }),
  searchDir: (id: string, q: string, limit: number, offset: number) =>
    http<SearchPage>(`/dirs/${enc(id)}/search`, { query: { q, limit, offset } }),
  item: (id: string) => http<ItemDetail>(`/items/${enc(id)}`),
  itemContent: (id: string) => http<string, 'text'>(`/items/${enc(id)}/content`, { responseType: 'text' }),

  admin: {
    login: (username: string, password: string) => http('/admin/login', { method: 'POST', body: { username, password } }),
    logout: () => http('/admin/logout', { method: 'POST' }),
    me: () => http<{ admin: boolean }>('/admin/me'),
    tree: () => http<AdminTree>('/admin/tree'),
    createDir: (body: { parent_id: string | null; name: string; description?: string; password?: string }) =>
      http<{ id: string }>('/admin/dirs', { method: 'POST', body }),
    patchDir: (id: string, patch: DirPatch) => http(`/admin/dirs/${enc(id)}`, { method: 'PATCH', body: patch }),
    deleteDir: (id: string) => http(`/admin/dirs/${enc(id)}`, { method: 'DELETE' }),
    setMounts: (id: string, dirs: string[]) => http(`/admin/items/${enc(id)}/mounts`, { method: 'PUT', body: { dirs } }),
    deleteItem: (id: string) => http(`/admin/items/${enc(id)}`, { method: 'DELETE' }),
    search: (p: AdminSearchParams, limit: number, offset: number) =>
      http<SearchPage>('/admin/search', {
        query: { q: p.q, type: p.type || undefined, from: p.from || undefined, to: p.to || undefined, dir: p.dir || undefined, unmounted: p.unmounted ? 1 : undefined, limit, offset },
      }),
  },
}
