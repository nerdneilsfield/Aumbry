import type { DirNode } from '@/api/types'

export interface FlatDir {
  id: string
  name: string
  depth: number
}

/** Depth-first flatten, optionally skipping a whole subtree. */
export function flattenTree(nodes: DirNode[], skipId?: string, depth = 0): FlatDir[] {
  return nodes.flatMap((n) =>
    n.id === skipId ? [] : [{ id: n.id, name: n.name, depth }, ...flattenTree(n.children, skipId, depth + 1)],
  )
}

export function findNode(nodes: DirNode[], id: string | null): DirNode | undefined {
  for (const n of nodes) {
    if (n.id === id) return n
    const hit = findNode(n.children, id)
    if (hit) return hit
  }
}

export function findParentId(nodes: DirNode[], id: string, parent: string | null = null): string | null | undefined {
  for (const n of nodes) {
    if (n.id === id) return parent
    const hit = findParentId(n.children, id, n.id)
    if (hit !== undefined) return hit
  }
}

/** Whether node `id` itself or any ancestor has a password set. */
export function isDirProtected(nodes: DirNode[], id: string | null, ancestorProtected = false): boolean {
  if (!id) return false
  for (const n of nodes) {
    const curProtected = ancestorProtected || n.has_password
    if (n.id === id) return curProtected
    if (findNode(n.children, id)) return isDirProtected(n.children, id, curProtected)
  }
  return false
}
