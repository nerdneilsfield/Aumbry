import * as v from 'valibot'
// Type-only import: erased at build time, so echarts stays out of the main chunks.
import type { EChartsCoreOption } from 'echarts/core'

/**
 * <chart> payload, PRD 6.3. We never pass the document's JSON to ECharts as-is: it is validated against this
 * schema and an option object is built from the checked fields only: the document never supplies a raw option or any
 * function. (The error-bar renderItem below is our own code, following ECharts' official custom-error-bar example.)
 */
const Num = v.pipe(v.number(), v.finite())
const NonNeg = v.pipe(Num, v.minValue(0, 'err 必须是非负数'))
const Series = (data: v.GenericSchema) =>
  v.pipe(
    v.array(v.object({ name: v.pipe(v.string(), v.nonEmpty('series[].name 不能为空')), data: v.array(data), err: v.optional(v.array(NonNeg)) })),
    v.minLength(1, 'series 至少 1 个'),
    v.maxLength(20, 'series 最多 20 个'),
  )
const labels = { x_label: v.optional(v.string()), y_label: v.optional(v.string()) }

const Categorical = v.pipe(
  v.object({ categories: v.pipe(v.array(v.string()), v.nonEmpty('categories 不能为空')), series: Series(Num), ...labels }),
  v.check((d) => d.series.reduce((n, s) => n + s.data.length, 0) <= 2000, '数据点总数不能超过 2000'),
)
const Scatter = v.pipe(
  v.object({ series: Series(v.tuple([Num, Num])), ...labels }),
  v.check((d) => d.series.reduce((n, s) => n + s.data.length, 0) <= 2000, '数据点总数不能超过 2000'),
)

export type ChartType = 'bar' | 'line' | 'pie' | 'scatter'
export const isChartType = (t: string): t is ChartType => ['bar', 'line', 'pie', 'scatter'].includes(t)

type CategoricalData = v.InferOutput<typeof Categorical>
type ScatterData = v.InferOutput<typeof Scatter>

export interface ChartSpec {
  option: EChartsCoreOption
  /** px; known before echarts loads so the placeholder reserves exactly this space */
  height: number
}

/** Parses + validates; returns a ready-to-render spec or a human readable error (with JSON path of the first issue). */
export function buildChart(type: string, json: string, dark: boolean): ChartSpec | { error: string } {
  if (!isChartType(type)) return { error: `不支持的图表类型 “${type}”（应为 bar / line / pie / scatter）` }
  let raw: unknown
  try {
    raw = JSON.parse(json)
  } catch (e) {
    return { error: `JSON 解析失败：${e instanceof Error ? e.message : String(e)}` }
  }
  const r = v.safeParse(type === 'scatter' ? Scatter : Categorical, raw)
  if (!r.success) {
    const i = r.issues[0]
    return { error: `数据不符合格式：${v.getDotPath(i) ?? '$'} — ${i.message}` }
  }
  const bad = crossCheck(type, r.output as CategoricalData)
  if (bad) return { error: `数据不符合格式：${bad.path} — ${bad.message}` }
  return type === 'scatter' ? scatterOption(r.output as ScatterData, dark) : categoricalOption(type, r.output as CategoricalData, dark)
}

/** Cross-field rules JSON Schema cannot express (PRD 6.3), reported with the JSON path of the offending field. */
function crossCheck(type: ChartType, d: { categories?: string[]; series: { data: unknown[]; err?: number[] }[] }) {
  for (const [i, s] of d.series.entries()) {
    if (s.err && type !== 'bar' && type !== 'line') return { path: `series[${i}].err`, message: 'err（误差棒）只适用于 bar / line' }
    if (d.categories && s.data.length !== d.categories.length) return { path: `series[${i}].data`, message: `长度 ${s.data.length} 必须等于 categories 的长度 ${d.categories.length}` }
    if (s.err && s.err.length !== s.data.length) return { path: `series[${i}].err`, message: `长度 ${s.err.length} 必须等于 data 的长度 ${s.data.length}` }
  }
}

const PALETTE = ['#3b82f6', '#10b981', '#f59e0b', '#a855f7', '#ef4444', '#06b6d4', '#ec4899', '#84cc16']

function tone(dark: boolean) {
  return {
    text: dark ? '#d1d5db' : '#374151',
    muted: dark ? '#9ca3af' : '#4b5563',
    axis: dark ? '#4b5563' : '#9ca3af',
    split: dark ? '#1f2937' : '#e5e7eb',
    tipBg: dark ? '#111827' : '#ffffff',
    tipBorder: dark ? '#374151' : '#d1d5db',
  }
}

function base(dark: boolean, trigger: 'axis' | 'item', legend: boolean) {
  const t = tone(dark)
  return {
    color: PALETTE,
    backgroundColor: 'transparent',
    textStyle: { color: t.text, fontFamily: 'Inter, ui-sans-serif, system-ui, "PingFang SC", "Noto Sans SC", sans-serif' },
    // richText: the tooltip is drawn on the canvas as text. No HTML string is ever built from series names or categories.
    tooltip: {
      trigger,
      renderMode: 'richText',
      confine: true,
      backgroundColor: t.tipBg,
      borderColor: t.tipBorder,
      textStyle: { color: t.text, fontSize: 12 },
    },
    legend: legend ? { type: 'scroll', top: 0, textStyle: { color: t.text }, pageTextStyle: { color: t.text }, pageIconColor: t.text } : undefined,
  } satisfies EChartsCoreOption
}

function categoricalOption(type: 'bar' | 'line' | 'pie', d: CategoricalData, dark: boolean): ChartSpec {
  const t = tone(dark)
  const n = d.categories.length
  const longest = Math.max(...d.categories.map((c) => [...c].length))

  if (type === 'pie') {
    const s = d.series[0]
    return {
      height: 360,
      option: {
        ...base(dark, 'item', false),
        legend: { type: 'scroll', bottom: 0, textStyle: { color: t.text }, pageTextStyle: { color: t.text }, pageIconColor: t.text },
        series: [{
          type: 'pie',
          name: s.name,
          radius: ['36%', '64%'],
          center: ['50%', '45%'],
          avoidLabelOverlap: true,
          itemStyle: { borderColor: dark ? '#111827' : '#ffffff', borderWidth: 2, borderRadius: 4 },
          label: { color: t.text, formatter: '{b}\n{d}%', width: 120, overflow: 'truncate' },
          labelLine: { lineStyle: { color: t.axis } },
          data: d.categories.map((name, i) => ({ name, value: s.data[i] })),
        }],
      },
    }
  }

  // Horizontal bars keep long category names readable; the full name is in the tooltip.
  const hasErr = d.series.some((s) => s.err)
  // (error bars are drawn for vertical layouts only)
  const horizontal = type === 'bar' && !hasErr && (longest > 10 || n > 8)
  const catAxis = {
    type: 'category' as const,
    data: d.categories,
    inverse: horizontal,
    axisLine: { lineStyle: { color: t.axis } },
    axisTick: { show: false },
    axisLabel: horizontal
      ? { color: t.text, width: 130, overflow: 'truncate' as const, interval: 0 }
      : { color: t.text, interval: 0, width: n > 6 ? 70 : 110, overflow: n > 6 ? ('truncate' as const) : ('break' as const), rotate: n > 6 ? 30 : 0 },
    name: horizontal ? undefined : d.x_label,
    nameLocation: 'middle' as const,
    nameGap: n > 6 ? 62 : 34,
    nameTextStyle: { color: t.muted },
  }
  const valAxis = {
    type: 'value' as const,
    axisLabel: { color: t.muted },
    splitLine: { lineStyle: { color: t.split } },
    name: horizontal ? d.x_label : d.y_label,
    nameLocation: 'end' as const,
    nameTextStyle: { color: t.muted, align: horizontal ? ('right' as const) : ('left' as const) },
  }
  const labelled = d.series.length === 1 && n <= 12 && !hasErr
  const series = d.series.map((s) => ({
    type,
    name: s.name,
    data: s.data,
    ...(type === 'line' ? { smooth: false, symbolSize: 7, lineStyle: { width: 2.5 } } : { barMaxWidth: 36, itemStyle: { borderRadius: horizontal ? [0, 4, 4, 0] : [4, 4, 0, 0] } }),
    label: labelled ? { show: true, position: horizontal ? ('right' as const) : ('top' as const), color: t.text, fontSize: 11 } : { show: false },
  }))

  // Error bars: a custom series per series with `err`, reading [category, mean - err, mean + err] (official errorbar pattern).
  const errSeries = d.series.flatMap((s, k) =>
    s.err
      ? [{
          type: 'custom' as const,
          name: `${s.name} ±误差`,
          silent: true,
          tooltip: { show: false },
          z: 10,
          itemStyle: { color: PALETTE[k % PALETTE.length] },
          encode: { x: 0, y: [1, 2] },
          data: (s.data as number[]).map((m, i) => [i, m - s.err![i], m + s.err![i]]),
          renderItem: (_p: unknown, api: CustomApi) => {
            const i = api.value(0)
            const hi = api.coord([i, api.value(2)])
            const lo = api.coord([i, api.value(1)])
            const band = api.size([1, 0])[0]
            // offset of this series' bar inside its category group (default barGap 30%, barCategoryGap 20%, barMaxWidth 36)
            const N = d.series.length
            const w = Math.min((band * 0.8) / (N + 0.3 * (N - 1)), 36)
            const x = hi[0] + (type === 'bar' ? (k - (N - 1) / 2) * w * 1.3 : 0)
            const cap = Math.max(Math.min(w * 0.3, 8), 3)
            const style = api.style({ stroke: t.text, fill: undefined, lineWidth: 1.6 })
            return {
              type: 'group',
              children: [
                { type: 'line', shape: { x1: x, y1: hi[1], x2: x, y2: lo[1] }, style },
                { type: 'line', shape: { x1: x - cap, y1: hi[1], x2: x + cap, y2: hi[1] }, style },
                { type: 'line', shape: { x1: x - cap, y1: lo[1], x2: x + cap, y2: lo[1] }, style },
              ],
            }
          },
        }]
      : [],
  )

  return {
    height: horizontal ? Math.max(260, 90 + n * 34 * d.series.length) : n > 6 ? 410 : 350,
    option: {
      ...base(dark, 'axis', d.series.length > 1),
      // the error-bar custom series stay out of the legend
      legend: d.series.length > 1 ? { type: 'scroll', top: 0, data: d.series.map((x) => x.name), textStyle: { color: t.text }, pageTextStyle: { color: t.text }, pageIconColor: t.text } : undefined,
      grid: { top: d.series.length > 1 ? 52 : 36, left: 12, right: horizontal ? 36 : 20, bottom: !horizontal && d.x_label ? (n > 6 ? 40 : 28) : 12, containLabel: true },
      xAxis: horizontal ? valAxis : catAxis,
      yAxis: horizontal ? catAxis : valAxis,
      series: [...series, ...errSeries],
    },
  }
}

/** the slice of ECharts' custom-series `api` that renderItem uses */
interface CustomApi {
  value: (dim: number) => number
  coord: (v: number[]) => number[]
  size: (v: number[]) => number[]
  style: (o: Record<string, unknown>) => Record<string, unknown>
}

function scatterOption(d: ScatterData, dark: boolean): ChartSpec {
  const t = tone(dark)
  const axis = (name: string | undefined, loc: 'middle' | 'end') => ({
    type: 'value' as const,
    scale: true,
    name,
    nameLocation: loc,
    nameGap: 30,
    nameTextStyle: { color: t.muted, align: loc === 'end' ? ('left' as const) : ('center' as const) },
    axisLine: { lineStyle: { color: t.axis } },
    axisLabel: { color: t.muted },
    splitLine: { lineStyle: { color: t.split } },
  })
  return {
    height: 360,
    option: {
      ...base(dark, 'item', d.series.length > 1),
      grid: { top: d.series.length > 1 ? 64 : 40, left: 12, right: 24, bottom: d.x_label ? 30 : 12, containLabel: true },
      xAxis: axis(d.x_label, 'middle'),
      yAxis: axis(d.y_label, 'end'),
      series: d.series.map((s) => ({ type: 'scatter' as const, name: s.name, data: s.data, symbolSize: 11, itemStyle: { opacity: 0.85 } })),
    },
  }
}
