/** Formats a byte count using base-1024 units (`0 B`, `1.50 KB`, `12.3 MB`, `2.40 GB`). */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${Math.floor(bytes)} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let val = bytes / 1024
  let idx = 0
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx++
  }
  const decimals = val >= 100 ? 0 : val >= 10 ? 1 : 2
  return `${val.toFixed(decimals)} ${units[idx]}`
}

/** Formats an uptime duration in seconds (`45s`, `12m 30s`, `3h 15m`, `2d 4h 10m`). */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return '0s'
  const total = Math.floor(seconds)
  const d = Math.floor(total / 86400)
  const h = Math.floor((total % 86400) / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

/** Formats a non-negative CPU usage percentage with one decimal digit. */
export function formatCpuPercent(percent: number): string {
  if (!Number.isFinite(percent) || percent < 0) return '0.0%'
  return `${percent.toFixed(1)}%`
}
