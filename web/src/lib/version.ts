import type { ServerVersion } from '@/api/types'

/** Prefixes semver-like numeric versions with `v` while keeping `dev` or pre-prefixed tags intact. */
export function formatVersionTag(version: string): string {
  const trimmed = version.trim()
  if (!trimmed) return 'dev'
  return /^\d/.test(trimmed) ? `v${trimmed}` : trimmed
}

/** Formats an ISO build timestamp as `YYYY-MM-DD HH:mm UTC`, falling back to raw text when unparseable. */
export function formatBuildTime(buildTime: string): string {
  const trimmed = buildTime.trim()
  if (!trimmed || trimmed === 'unknown') return 'unknown'
  const ms = Date.parse(trimmed)
  if (Number.isNaN(ms)) return trimmed
  const iso = new Date(ms).toISOString()
  return `${iso.slice(0, 10)} ${iso.slice(11, 16)} UTC`
}

/** Normalizes server build metadata for display in the web footer. */
export function formatVersionSummary(info: ServerVersion) {
  const rawBuildTime = info.build_time.trim() || 'unknown'
  return {
    version: formatVersionTag(info.version),
    commit: info.commit.trim() || 'none',
    buildTime: formatBuildTime(rawBuildTime),
    rawBuildTime,
  }
}
